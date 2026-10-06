"""Resume the full 17x6 + AES all-local matrix without replaying request windows.

Run locally under tmux with SSH agent forwarding. Dedicated worker unit only;
shared services and old corpus/cache/results are never stopped or cleared.
"""
import argparse
import csv
import fcntl
import json
import pathlib
import shlex
import subprocess
import sys
import time

from launcher import checked_config
from point_config import point_config
from prepare import publish
from runtime_probe import idle
from validate_point import audit, formal_plan, vm_lifecycles
from accounting_inputs import load_accounting, validate_accounting

HERE = pathlib.Path(__file__).resolve().parent
HOSTS = dict(worker="Liquidz@er020.utah.cloudlab.us", backend="Liquidz@er069.utah.cloudlab.us",
             loader="Liquidz@er032.utah.cloudlab.us")
SSH = ["ssh", "-A", "-oBatchMode=yes", "-oConnectTimeout=10", "-oServerAliveInterval=20",
       "-oServerAliveCountMax=6", "-oStrictHostKeyChecking=no"]
SCP = ["scp", "-C", "-p", "-oBatchMode=yes", "-oConnectTimeout=10"]
SYSTEM_ORDER = ["splitsnap-zstd3", "ws-zstd3", "no-image-zstd3", "full-dedup-zstd3",
                "chunks-128k-zstd3", "pages-4k-zstd3"]


def remote(role, *args):
    return subprocess.check_output(SSH + [HOSTS[role], shlex.join(map(str, args))], text=True)


def copy(role, source, target, upload=False):
    args = [str(source), HOSTS[role] + ":" + str(target)] if upload else [HOSTS[role] + ":" + str(source), str(target)]
    subprocess.run(SCP + args, check=True)


def collect_window(window, point):
    # Calls have finished. Bundle the many small records in one SSH stream;
    # this is evidence transfer, outside both the invocation and WS timers.
    command = shlex.join(["tar", "-czf", "-", "-C", window, "."])
    with subprocess.Popen(SSH + [HOSTS["loader"], command], stdout=subprocess.PIPE) as sender:
        extracted = subprocess.run(["tar", "-xzf", "-", "-C", str(point)], stdin=sender.stdout)
        sender.stdout.close()
        sent = sender.wait()
    if extracted.returncode or sent:
        raise RuntimeError(f"window collection failed (extract={extracted.returncode}, source={sent}); retain files, do not replay calls")


def collect_resources(point, unit):
    path = point / "relay-resources.json"
    if path.exists():
        if json.loads(path.read_text())["unit"] != unit:
            raise ValueError("resource record unit differs")
        return
    output = remote("worker", "sudo", "-n", "journalctl", "-u", unit, "--no-pager", "-o", "json",
                    "-g", "Consumed .* CPU time")
    records = [json.loads(line) for line in output.splitlines() if line.strip()]
    records = [r for r in records if r.get("MESSAGE", "").startswith(unit+": Consumed ")]
    if len(records) != 1 or "memory peak" not in records[0]["MESSAGE"]:
        raise ValueError("missing/ambiguous ended relay resource accounting; collect again, not the request window")
    publish(path, dict(unit=unit, message=records[0]["MESSAGE"],
                      realtime_us=records[0]["__REALTIME_TIMESTAMP"],
                      scope="Entire dedicated relay service lifetime, including startup, warmup, measured calls, teardown and collection wait; not per-call decompression and not guest/containerd-wide accounting. Memory is systemd's rounded service peak, not an exact RSS sample."))


def job_status(role, job, session):
    # A missing tmux handle is not permission to reissue a partially run window.
    command = (f"if test -f {shlex.quote(job + '/rc')}; then printf 'DONE '; cat {shlex.quote(job + '/rc')}; "
               f"elif tmux has-session -t {shlex.quote(session)} 2>/dev/null; then "
               f"tmux list-panes -t {shlex.quote(session)} -F 'RUNNING #{{pane_pid}} #{{pane_dead}}'; "
               f"elif test -e {shlex.quote(job)}; then echo UNKNOWN; else echo ABSENT; fi")
    return remote(role, "bash", "-c", command).strip()


def wait_job(role, job, session):
    deadline = time.monotonic() + 1800
    while True:
        status = job_status(role, job, session)
        if status.startswith("DONE "):
            return int(status.split()[1])
        if not status.startswith("RUNNING ") or status.split()[-1] != "0":
            raise RuntimeError("job has no verified live handle or terminal rc; inspect, do not replay: " + status)
        if time.monotonic() >= deadline:
            raise RuntimeError("observation deadline only; reobserve the same job: " + job)
        time.sleep(5)


def ordered_points(plan):
    profiles = [w["profile"] for w in plan["workloads"]["workloads"]]
    profiles.sort(key=lambda p: p != "aes-go-45000-45450")
    return [(s, p, "remote") for s in SYSTEM_ORDER for p in profiles] + [
        ("splitsnap-zstd3", "aes-go-45000-45450", "local")]


def require_versioned_negative_gate(gates, version):
    root = gates / ("aes-negative-" + version)
    validation = json.loads((root / "negative-validation.json").read_text())
    launch = json.loads((root / "launch.json").read_text())["config"]
    final = json.loads((root / "final.json").read_text())
    if (not validation["accepted"] or not idle(final["runtime"]) or
            pathlib.Path(launch["relay_binary"]).name != "relay-streams8-" + version or
            final["binary"] != launch["relay_binary"]):
        raise ValueError("negative gate binary/release differs from this formal version")
    if version in ("gate11", "gate12"):
        shutdown = json.loads((root / "shutdown.json").read_text())
        late = json.loads((gates / ("aes-cancel-after-load-" + version) / "validation.json").read_text())
        if (shutdown["binary"] != launch["relay_binary"] or not shutdown["processes_absent"] or
                shutdown["remaining_processes"] or validation["released_vm"] not in shutdown["vm_ids"] or
                late["binary"] != launch["relay_binary"] or not late["accepted"] or
                not late["cancelled_after_load"] or not late["uffd_released"] or not late["runtime_idle"] or
                late["vm_id"] not in shutdown["vm_ids"]):
            raise ValueError("late-cancellation/pool shutdown evidence incomplete")


class Matrix:
    def __init__(self, root, version):
        self.root, self.version = root, version
        self.config_root = HERE / "config/20260910-r1"
        self.provenance = HERE / "provenance/20260910-r1"
        self.plan = json.loads((self.config_root / "plan.json").read_text())
        self.inventory = json.loads((self.provenance / "actual-layout.json").read_text())
        self.aliases = json.loads((self.provenance / "aliases-complete.json").read_text())
        if self.plan["run_id"] != "20260910-r1" or self.plan["backend"] != "10.0.1.2":
            raise ValueError("this deployment is the fixed three-node currentbase17 environment")
        if len(self.inventory["rows"]) != 102 or len(self.aliases["aliases"]) != 5100 or not self.aliases["materialized"]:
            raise ValueError("actual full corpus/aliases required")
        self.remote_root = "/users/Liquidz/streams8/20260910-r1"
        self.toolset = self.remote_root + "/toolset-" + version
        self.root.mkdir(parents=True, exist_ok=True)
        self.lock = (self.root / "controller.lock").open("a")
        fcntl.flock(self.lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        accounting = load_accounting(self.provenance)
        validate_accounting(accounting, self.inventory)
        frozen = dict(plan=self.plan, inventory=self.inventory, aliases=self.aliases,
                      accounting=accounting,
                      version=version, points=ordered_points(self.plan), hosts=HOSTS,
                      calls=60, warmup_slots=list(range(30)), measured_slots=list(range(30, 60)),
                      cadence="absolute 1 RPS; HTTP and teardown overlap recorded, not assumed isolated",
                      page_cache_policy="no system-wide drop; fresh per-point worker cache, 30 warmup slots")
        if version in ("gate11", "gate12"):
            self.release = json.loads((self.provenance / (version + "-release.json")).read_text())
            frozen["release"] = self.release
        target = self.root / "frozen.json"
        if target.exists():
            if json.loads(target.read_text()) != json.loads(json.dumps(frozen)):
                raise ValueError("frozen inputs/version changed; choose a separate result namespace")
        else:
            publish(target, frozen)

    def probe(self, remote_config, action):
        return json.loads(remote("worker", "sudo", "-n", "python3", self.toolset + "/point_worker.py", remote_config, action))

    def platform(self):
        return {role: json.loads(remote(role, "python3", self.toolset + "/platform_probe.py", role)) for role in HOSTS}

    def stop_and_collect(self, point, c, remote_config):
        stopped = json.loads(remote("worker", "sudo", "-n", "python3", self.toolset + "/launcher.py", remote_config, "stop"))
        if self.version in ("gate11", "gate12"):
            if stopped["unit"] != c["unit"] or not stopped["processes_absent"] or stopped["remaining_processes"]:
                raise ValueError("owned VMM shutdown evidence incomplete")
            if not (point / "shutdown.json").exists():
                publish(point / "shutdown.json", stopped)
            copy("worker", c["log_dir"] + "/relay.log", point / "relay-shutdown.log")
        collect_resources(point, c["unit"])

    def collect_final(self, point, c, remote_config):
        # Preserve the evidence even when idle fails. A timeout is not permission
        # to stop a VM, reset counters or replay a request window.
        copy("worker", c["log_dir"] + "/relay.log", point / "relay-before-final.log")
        copy("worker", c["log_dir"] + "/launch.json", point / "launch.json")
        try:
            return self.probe(remote_config, "final")
        except Exception as error:
            publish(point / f"collection-error-{time.time_ns()}.json", dict(error=str(error),
                    action="observe existing job; do not replay", captured_at=time.time()))
            try:
                diagnostic = self.probe(remote_config, "diagnostic")
            except Exception as diagnostic_error:
                diagnostic = dict(diagnostic_only=True, error=str(diagnostic_error))
            publish(point / f"unsettled-diagnostic-{time.time_ns()}.json", diagnostic)
            raise
        finally:
            copy("worker", c["log_dir"] + "/relay.log", point / "relay.log")

    def verify_prerequisites(self):
        if self.version in ("gate11", "gate12"):
            binary = self.remote_root + "/bin/relay-streams8-" + self.version
            actual = remote("worker", "sha256sum", binary).split()[0]
            if actual != self.release["relay_sha256"]:
                raise ValueError("deployed relay differs from frozen release; do not reuse negative evidence")
        # CPU-intensive static accounting must not overlap measured runtime.
        for name in ("prepare-job", "views-job", "inventory-job", "aliases-job", "footprint-job", "payload-job"):
            if remote("backend", "cat", self.remote_root + "/" + name + "/rc").strip() != "0":
                raise RuntimeError("preparation/accounting not complete: " + name)
        gates = HERE / "verification/remote-20260910-r1"
        for name in ("aes-gate2", "imagepy-large-gate1", "aes-local-gate3"):
            result = json.loads((gates / name / "validation.json").read_text())
            if result["calls"] != 1 or result["layout"] != "streams8-v1":
                raise ValueError("missing functional gate: " + name)
        if not idle(json.loads((gates / "aes-local-gate3/runtime-state-final.json").read_text())):
            raise ValueError("local gate did not release UFFD/VM")
        if self.version in ("gate10", "gate11", "gate12"):
            require_versioned_negative_gate(gates, self.version)
        elif not json.loads((gates / "aes-negative-gate2/negative-validation.json").read_text())["accepted"]:
            raise ValueError("missing required-WS failure proof")

    def point(self, system, profile, mode):
        point = self.root / "points" / (system + "--" + profile + "--" + mode)
        point.mkdir(parents=True, exist_ok=True)
        c = point_config(self.plan, system, profile, "formal-" + self.version + "-r1", mode)
        c["relay_binary"] = self.remote_root + "/bin/relay-streams8-" + self.version
        c = checked_config(c)
        config = point / "config.json"
        if config.exists():
            if json.loads(config.read_text()) != c:
                raise ValueError("point config changed")
        else:
            publish(config, c)
        remote_config = self.toolset + "/" + c["point_id"] + ".json"
        if (point / "POINT_COMPLETE.json").exists():
            self.validate(point, c, system, profile, mode)
            stage = json.loads(remote("worker", "sudo", "-n", "python3", self.toolset + "/launcher.py", remote_config, "status"))
            if stage.get("ActiveState") == "active":
                self.probe(remote_config, "final")
            self.stop_and_collect(point, c, remote_config)
            print("POINT_RECORDED", system, profile, mode, flush=True)
            return
        print("POINT_BEGIN", system, profile, mode, flush=True)
        copy("worker", config, remote_config, upload=True)
        stage = json.loads(remote("worker", "sudo", "-n", "python3", self.toolset + "/launcher.py", remote_config, "status"))
        if stage.get("LoadState") == "not-found":
            if (point / "started.json").exists():
                raise RuntimeError("previous unit disappeared; do not replay this point")
            if mode == "local":
                report = self.remote_root + "/" + c["point_id"] + "-local-preparation.json"
                remote("worker", "sudo", "-n", "python3", self.toolset + "/local_aliases.py", "--config", remote_config,
                       "--source", self.remote_root + "/local-cache-gate2.json",
                       "--aliases", self.toolset + "/aliases-complete.json", "--report", report)
                copy("worker", report, point / "local-preparation.json")
            before = self.platform()
            if not (point / "platform-before.json").exists():
                publish(point / "platform-before.json", before)
            started = json.loads(remote("worker", "sudo", "-n", "python3", self.toolset + "/launcher.py", remote_config, "start"))
            publish(point / "started.json", started)
        elif stage.get("ActiveState") != "active":
            raise RuntimeError("existing unit not active; inspect, do not restart: " + repr(stage))
        ready = self.probe(remote_config, "ready")
        if not (point / "ready.json").exists():
            publish(point / "ready.json", ready)
        job = self.remote_root + "/formal-jobs/" + c["point_id"]
        session = "streams8_" + c["point_id"]
        window = self.remote_root + "/formal-windows/" + c["point_id"]
        status = job_status("loader", job, session)
        w = next(w for w in self.plan["workloads"]["workloads"] if w["profile"] == profile)
        if status == "ABSENT":
            reset = self.probe(remote_config, "reset")
            if not (point / "counter-reset.json").exists():
                publish(point / "counter-reset.json", reset)
            args = ["bash", self.toolset + "/run-direct-window.sh", c["relay_endpoint"], self.plan["backend"],
                    window, "60", "1000", w["snapshot"], self.aliases["tag"], profile,
                    self.toolset + "/workloads.json", self.toolset + "/direct_requests.json"]
            remote("loader", "bash", self.remote_root + "/config/run_detached_experiment_job.sh",
                   job, session, "--", *args)
        elif status == "UNKNOWN":
            raise RuntimeError("window state unknown; never restart just because SSH/tmux vanished")
        rc = wait_job("loader", job, session)
        # Always retain failed calls too, before deciding whether the point passes.
        for name in ("stdout.log", "stderr.log", "rc"):
            copy("loader", job + "/" + name, point / ("window-" + name))
        collect_window(window, point)
        final = self.collect_final(point, c, remote_config)
        for name, value in (("runtime-state-final.json", final["runtime"]),
                            ("remote-fetch-stats.json", final["fetch_stats"]), ("final.json", final)):
            if not (point / name).exists():
                publish(point / name, value)
        if rc != 0:
            raise RuntimeError("window failed; raw evidence retained: " + str(point))
        after = self.platform()
        if not (point / "platform-after.json").exists():
            publish(point / "platform-after.json", after)
        result = self.validate(point, c, system, profile, mode)
        if not (point / "validation.json").exists():
            publish(point / "validation.json", result)
        publish(point / "POINT_COMPLETE.json", dict(system=system, profile=profile, mode=mode, version=self.version,
                                                   calls=60, measured=30, finished_at=time.time()))
        # This exact owned unit is idle and collected. Never stop shared services.
        self.stop_and_collect(point, c, remote_config)
        print("POINT_COMPLETE", system, profile, mode, flush=True)

    def validate(self, root, c, system, profile, mode):
        with (root / "invocations.tsv").open() as f:
            rows = list(csv.DictReader(f, delimiter="\t"))
        if any(r["exit_status"] != "0" or r["reply_ok"] != "1" or r["direct_native"] != "1" for r in rows):
            raise ValueError("unsuccessful/non-native calls")
        point, expected = formal_plan(rows, self.inventory, self.aliases, c, system, profile)
        prefix = point["snapshot"] + "-" + self.aliases["tag"] + "-"
        log = (root / "relay.log").read_text()
        result = audit(log, json.loads((root / "remote-fetch-stats.json").read_text()),
                       60, c["ws_coalescing"], prefix, mode, point.get("manifest"))
        if c["ws_coalescing"] and result["decoded_revisions"] != expected:
            raise ValueError("decoded revision set differs")
        if not idle(json.loads((root / "runtime-state-final.json").read_text())):
            raise ValueError("window runtime has not settled")
        result["lifecycle"] = vm_lifecycles(log, expected, c["ws_coalescing"])
        result.update(system=system, profile=profile, warmup_slots=list(range(30)), measured_slots=list(range(30, 60)))
        return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--version", choices=("gate7", "gate8", "gate9", "gate10", "gate11", "gate12"), default="gate12")
    p.add_argument("--only", help="one system:profile:mode; retained as part of the full matrix, not completion")
    a = p.parse_args()
    runner = Matrix(a.output.resolve(), a.version)
    runner.verify_prerequisites()
    points = ordered_points(runner.plan)
    if a.only:
        selected = tuple(a.only.split(":"))
        if selected not in points:
            raise ValueError("not a formal matrix point")
        points = [selected]
    for point in points:
        runner.point(*point)
    if not a.only:
        publish(runner.root / "MEASUREMENTS_COMPLETE.json", dict(points=103, calls=6180,
                pending=["aggregate statistics", "new Figures9-13", "final artifact audit"]))


if __name__ == "__main__":
    main()

import json
import pathlib
import tempfile
import unittest

from local_aliases import materialize


class LocalAliasesTest(unittest.TestCase):
    def test_immutable_links_and_fresh_destination(self):
        with tempfile.TemporaryDirectory(prefix="streams8-test-") as directory:
            root = pathlib.Path(directory) / "streams8/run"
            src = root / "points/source/snapshots"
            snapshot = "cold-aes-go-45000-45450-test"
            keys = [snapshot + "/snap_file", snapshot + "/working_set_pages_content_private.zstd.streams", "base/recipe_file"]
            for key in keys + ["_chunks_zstd_v1_l3/00/0011"]:
                path = src / key
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(b"immutable")
            cfg = dict(run_id="run", point_id="aes-local", run_root=str(root),
                       cache_dir=str(root / "points/destination/snapshots"), scratch_dir=str(root / "points/destination/scratch"),
                       log_dir=str(root / "points/destination/logs"), relay_binary="/bin/relay", relay_cwd="/relay", images_dir="/images",
                       network_name_prefix="sstr", relay_endpoint="10.0.1.1:8090", ws_cache_endpoint="10.0.1.1:8091",
                       mode="local", security="partial", ws_coalescing=True, vm_mib=512, chunk_size=4096, clean=False,
                       minio_endpoint="10.0.1.2:9565")
            source = dict(cache=str(src), snapshot=snapshot, source_endpoint=cfg["minio_endpoint"], metadata_and_ws_files=keys, native_chunks=1)
            aliases = dict(run_id="run", layout="streams8-v1", materialized=True, tag="streams8-run", aliases=[
                dict(endpoint=cfg["minio_endpoint"], source_revision=snapshot, slot=i, alias_revision=f"{snapshot}-streams8-run-{i}",
                     source_entries=[dict(key=k, size=9) for k in keys if k.startswith(snapshot + "/")]) for i in range(60)])
            result = materialize(cfg, source, aliases)
            self.assertEqual(result["aliases"], 60)
            original = src / keys[1]
            linked = pathlib.Path(cfg["cache_dir"]) / (snapshot + "-streams8-run-59") / original.name
            self.assertEqual(original.stat().st_ino, linked.stat().st_ino)
            self.assertEqual(original.read_bytes(), b"immutable")
            with self.assertRaises(FileExistsError):
                materialize(cfg, source, aliases)
            aliases["materialized"] = False
            with self.assertRaises(ValueError):
                materialize(cfg, source, aliases)

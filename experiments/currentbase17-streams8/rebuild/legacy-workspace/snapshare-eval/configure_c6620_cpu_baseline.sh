#!/usr/bin/env bash
# Configure/restore the SplitSnap c6620 CPU baseline. The apply action records
# the original state before disabling SMT and fixing every physical core at the
# Xeon Gold 5512U base frequency (2.1 GHz).
set -euo pipefail

target_khz=2100000

die() {
  echo "ERROR: $*" >&2
  exit 1
}

read_optional() {
  local path=$1
  local value
  if [[ -r $path ]] && value=$(cat "$path" 2>/dev/null); then
    printf '%s' "$value"
  else
    printf 'NA'
  fi
}

policy_dirs() {
  local policy hardware_max
  while IFS= read -r policy; do
    [[ -r $policy/scaling_max_freq && -r $policy/cpuinfo_max_freq ]] || continue
    hardware_max=$(cat "$policy/cpuinfo_max_freq" 2>/dev/null || true)
    [[ $hardware_max =~ ^[0-9]+$ ]] || continue
    printf '%s\n' "$policy"
  done < <(find /sys/devices/system/cpu/cpufreq -maxdepth 1 -type d -name 'policy*' -print | sort -V)
}

show_status() {
  echo "timestamp=$(date -Is)"
  echo "hostname=$(hostname)"
  echo "online_cpus=$(read_optional /sys/devices/system/cpu/online)"
  echo "smt_control=$(read_optional /sys/devices/system/cpu/smt/control)"
  echo "smt_active=$(read_optional /sys/devices/system/cpu/smt/active)"
  echo "intel_pstate_no_turbo=$(read_optional /sys/devices/system/cpu/intel_pstate/no_turbo)"
  echo "intel_pstate_hwp_dynamic_boost=$(read_optional /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost)"
  echo -e 'policy\tgovernor\tepp\tmin_khz\tmax_khz\tbase_khz\tcpuinfo_min_khz\tcpuinfo_max_khz'
  local policy
  while IFS= read -r policy; do
    echo -e "$(basename "$policy")\t$(read_optional "$policy/scaling_governor")\t$(read_optional "$policy/energy_performance_preference")\t$(read_optional "$policy/scaling_min_freq")\t$(read_optional "$policy/scaling_max_freq")\t$(read_optional "$policy/base_frequency")\t$(read_optional "$policy/cpuinfo_min_freq")\t$(read_optional "$policy/cpuinfo_max_freq")"
  done < <(policy_dirs)
}

capture_state() {
  local state_dir=$1
  [[ ! -e $state_dir/capture.complete ]] || die "state already captured: $state_dir"
  mkdir -p "$state_dir"
  show_status > "$state_dir/cpu-state.before.txt"
  lscpu > "$state_dir/lscpu.before.txt"
  {
    echo -e 'policy\tgovernor\tepp\tmin_khz\tmax_khz'
    local policy
    while IFS= read -r policy; do
      echo -e "$(basename "$policy")\t$(read_optional "$policy/scaling_governor")\t$(read_optional "$policy/energy_performance_preference")\t$(read_optional "$policy/scaling_min_freq")\t$(read_optional "$policy/scaling_max_freq")"
    done < <(policy_dirs)
  } > "$state_dir/policies.before.tsv"
  {
    echo "smt_control=$(read_optional /sys/devices/system/cpu/smt/control)"
    echo "no_turbo=$(read_optional /sys/devices/system/cpu/intel_pstate/no_turbo)"
    echo "hwp_dynamic_boost=$(read_optional /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost)"
  } > "$state_dir/globals.before.env"
  date -Is > "$state_dir/capture.complete"
}

verify_fixed() {
  local count=0 policy
  [[ $(read_optional /sys/devices/system/cpu/smt/active) == 0 ]] || die "SMT is still active"
  [[ $(read_optional /sys/devices/system/cpu/intel_pstate/no_turbo) == 1 ]] || die "Turbo is still enabled"
  while IFS= read -r policy; do
    ((count += 1))
    [[ $(read_optional "$policy/scaling_governor") == performance ]] || die "$policy governor mismatch"
    [[ $(read_optional "$policy/scaling_min_freq") == $target_khz ]] || die "$policy min mismatch"
    [[ $(read_optional "$policy/scaling_max_freq") == $target_khz ]] || die "$policy max mismatch"
  done < <(policy_dirs)
  [[ $count -eq 28 ]] || die "expected 28 active cpufreq policies, found $count"
}

apply_fixed() {
  local state_dir=$1
  if [[ -f $state_dir/capture.complete ]]; then
    [[ ! -f $state_dir/apply.complete ]] || die "2.1-GHz baseline already applied: $state_dir"
    echo "Resuming incomplete apply using captured state: $state_dir" >&2
  else
    capture_state "$state_dir"
  fi
  [[ -w /sys/devices/system/cpu/smt/control ]] || die "SMT control unavailable"
  echo off > /sys/devices/system/cpu/smt/control
  echo 1 > /sys/devices/system/cpu/intel_pstate/no_turbo
  if [[ -w /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost ]]; then
    echo 0 > /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost
  fi
  local policy base_khz
  while IFS= read -r policy; do
    base_khz=$(read_optional "$policy/base_frequency")
    [[ $base_khz == $target_khz ]] || die "$policy base frequency is $base_khz, expected $target_khz"
    echo performance > "$policy/scaling_governor"
    if [[ -w $policy/energy_performance_preference ]]; then
      echo performance > "$policy/energy_performance_preference"
    fi
    echo "$target_khz" > "$policy/scaling_max_freq"
    echo "$target_khz" > "$policy/scaling_min_freq"
  done < <(policy_dirs)
  verify_fixed
  show_status > "$state_dir/cpu-state.after.txt"
  lscpu > "$state_dir/lscpu.after.txt"
  date -Is > "$state_dir/apply.complete"
  show_status
}

restore_state() {
  local state_dir=$1
  [[ -f $state_dir/capture.complete ]] || die "missing captured state: $state_dir"
  local saved_smt saved_no_turbo saved_hwp
  saved_smt=$(awk -F= '$1 == "smt_control" {print $2}' "$state_dir/globals.before.env")
  saved_no_turbo=$(awk -F= '$1 == "no_turbo" {print $2}' "$state_dir/globals.before.env")
  saved_hwp=$(awk -F= '$1 == "hwp_dynamic_boost" {print $2}' "$state_dir/globals.before.env")
  [[ $saved_smt != NA ]] && echo "$saved_smt" > /sys/devices/system/cpu/smt/control
  [[ $saved_no_turbo != NA ]] && echo "$saved_no_turbo" > /sys/devices/system/cpu/intel_pstate/no_turbo
  if [[ $saved_hwp != NA && -w /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost ]]; then
    echo "$saved_hwp" > /sys/devices/system/cpu/intel_pstate/hwp_dynamic_boost
  fi
  local name governor epp min_khz max_khz policy hardware_min hardware_max
  while IFS=$'\t' read -r name governor epp min_khz max_khz; do
    [[ $name != policy ]] || continue
    policy=/sys/devices/system/cpu/cpufreq/$name
    [[ -d $policy ]] || die "saved policy did not return: $name"
    hardware_min=$(<"$policy/cpuinfo_min_freq")
    hardware_max=$(<"$policy/cpuinfo_max_freq")
    echo "$hardware_max" > "$policy/scaling_max_freq"
    echo "$hardware_min" > "$policy/scaling_min_freq"
    [[ $governor == NA ]] || echo "$governor" > "$policy/scaling_governor"
    if [[ $epp != NA && -w $policy/energy_performance_preference ]]; then
      echo "$epp" > "$policy/energy_performance_preference"
    fi
    echo "$max_khz" > "$policy/scaling_max_freq"
    echo "$min_khz" > "$policy/scaling_min_freq"
  done < "$state_dir/policies.before.tsv"
  show_status > "$state_dir/cpu-state.restored.txt"
  date -Is > "$state_dir/restore.complete"
  show_status
}

case ${1:-} in
  status) show_status ;;
  verify-2.1ghz) verify_fixed; show_status ;;
  apply-2.1ghz)
    [[ $EUID -eq 0 ]] || die "run apply with sudo"
    [[ $# -eq 2 ]] || die "apply-2.1ghz requires STATE_DIR"
    apply_fixed "$2"
    ;;
  restore)
    [[ $EUID -eq 0 ]] || die "run restore with sudo"
    [[ $# -eq 2 ]] || die "restore requires STATE_DIR"
    restore_state "$2"
    ;;
  *) echo "usage: $0 {status|verify-2.1ghz|apply-2.1ghz STATE_DIR|restore STATE_DIR}" >&2; exit 2 ;;
esac

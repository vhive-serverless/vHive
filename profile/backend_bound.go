package profile

import "fmt"

func backendBound(values map[string]float64) float64 {
	// 1 - ( Frontend_Bound + Bad_Speculation + Retiring )
	values["Backend_Bound"] = 1 - (frontendBound(values) + badSpeculation(values) + retiring(values))
	return values["Backend_Bound"]
}

func beBoundAtEXE(values map[string]float64) float64 {
	// (CYCLE_ACTIVITY.CYCLES_NO_EXECUTE + UOPS_EXECUTED.THREAD:c1 -UOPS_EXECUTED.THREAD:c2) / CLOCKS
	// "events": ["cycle_activity.cycles_no_execute", "cpu/uops_executed.thread,cmask=1/", "cpu/uops_executed.thread,cmask=2/", "CLKS"]
	values["BE_Bound_at_EXE"] = backendBoundCycles(values) / clks(values)
	// values["BE_Bound_at_EXE"] = (values["cycle_activity.cycles_no_execute"] + values["cpu/uops_executed.thread,cmask=1/"] - values["cpu/uops_executed.thread,cmask=2/"]) / clks(values)
	return values["BE_Bound_at_EXE"]
}

/////////////////////////////////////////////////////////////////
////////////////////////// Memory Bound /////////////////////////
/////////////////////////////////////////////////////////////////
func memoryBound(values map[string]float64) float64 {
	// #Memory_Bound_Fraction * Backend_Bound
	// cycle_activity.stalls_ldm_pending / CLKS
	// values["Memory_Bound"] = values["cycle_activity.stalls_mem_any"] / clks(values)
	values["Memory_Bound"] = memoryBoundFraction(values) * backendBound(values)
	return values["Memory_Bound"]
}

func l1Bound(values map[string]float64) float64 {
	// max ( ( CYCLE_ACTIVITY.STALLS_MEM_ANY - CYCLE_ACTIVITY.STALLS_L1D_MISS ) / CLKS , 0 )
	// (CYCLE_ACTIVITY.STALLS_LDM_PENDING -CYCLE_ACTIVITY.STALLS_L1D_PENDING)/ CLOCKS
	// values["L1_Bound"] = (values["cycle_activity.stalls_ldm_pending"] - values["cycle_activity.stalls_l1d_pending"]) / clks(values)
	if val := (values["cycle_activity.stalls_mem_any"] - values["cycle_activity.stalls_l1d_miss"]) / clks(values); val > 0 {
		values["L1_Bound"] = val
	} else {
		values["L1_Bound"] = 0
	}
	return values["L1_Bound"]
}

func dtlbLoad(values map[string]float64) float64 {
	// ( #Mem_STLB_Hit_Cost * DTLB_LOAD_MISSES.STLB_HIT + DTLB_LOAD_MISSES.WALK_DURATION + 7 * DTLB_LOAD_MISSES.WALK_COMPLETED ) / CLKS
	values["DTLB_Load"] = (8*values["dtlb_load_misses.stlb_hit"] + values["cpu/dtlb_load_misses.walk_duration,cmask=1/"] + 7*values["dtlb_load_misses.walk_completed"]) / clks(values)
	return values["DTLB_Load"]
}

func fbFull(values map[string]float64) float64 {
	// Load_Miss_Real_Latency * L1D_PEND_MISS.FB_FULL / CLKS
	values["FB_Full"] = loadMissRealLatency(values) * values["cpu/l1d_pend_miss.fb_full,cmask=1/"] / clks(values)
	return values["FB_Full"]
}

func l2Bound(values map[string]float64) float64 {
	// ( CYCLE_ACTIVITY.STALLS_L1D_MISS - CYCLE_ACTIVITY.STALLS_L2_MISS ) / CLKS
	// (CYCLE_ACTIVITY.STALLS_L1D_PENDING - CYCLE_ACTIVITY.STALLS_L2_PENDING)/ CLOCKS
	values["L2_Bound"] = (values["cycle_activity.stalls_l1d_miss"] - values["cycle_activity.stalls_l2_miss"]) / clks(values)
	return values["L2_Bound"]
}

func l3Bound(values map[string]float64) float64 {
	// Mem_L3_Hit_Fraction * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	// CYCLE_ACTIVITY.STALLS_L2_PENDING * L3_Hit_fraction / CLOCKS
	values["L3_Bound"] = memL3HitFraction(values) * values["cycle_activity.stalls_l2_miss"] / clks(values)
	return values["L3_Bound"]
}

func dramBound(values map[string]float64) float64 {
	// ( 1 - Mem_L3_Hit_Fraction ) * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	// CYCLE_ACTIVITY.STALLS_L2_PENDING * L3_Miss_fraction / CLOCKS
	values["DRAM_Bound"] = (1 - memL3HitFraction(values)) * values["cycle_activity.stalls_l2_miss"] / clks(values)
	return values["DRAM_Bound"]
}

func memBandwidth(values map[string]float64) float64 {
	// #ORO_DRD_BW_Cycles / CLKS
	values["MEM_Bandwidth"] = oroDRDBWCycles(values) / clks(values)
	return values["MEM_Bandwidth"]
}

func memLatency(values map[string]float64) float64 {
	// #ORO_DRD_Any_Cycles / CLKS - MEM_Bandwidth
	values["MEM_Latency"] = oroDRDAnyCycles(values)/clks(values) - memBandwidth(values)
	return values["MEM_Latency"]
}

func uncoreBound(values map[string]float64) float64 {
	// CYCLE_ACTIVITY.STALLS_L2_PENDING / CLOCKS
	values["Uncore_Bound"] = values["cycle_activity.stalls_l2_miss"] / clks(values)
	return values["Uncore_Bound"]
}

func storeBound(values map[string]float64) float64 {
	// RESOURCE_STALLS.SB / CLKS
	values["Store_Bound"] = values["resource_stalls.sb"] / clks(values)
	return values["Store_Bound"]
}

func robBound(values map[string]float64) float64 {
	// resource_stalls.rob / CLKS
	values["ROB_Bound"] = values["resource_stalls.rob"] / clks(values)
	return values["ROB_Bound"]
}

func preciseLoadBreakdown(values map[string]float64) float64 {
	// [] / SumOf_PRECISE_LOADS
	loads := []string{"mem_load_uops_retired.hit_lfb", "mem_load_uops_retired.l1_hit", "mem_load_uops_retired.l2_hit", "mem_load_uops_retired.l3_hit", "mem_load_uops_l3_hit_retired.xsnp_miss", "mem_load_uops_l3_hit_retired.xsnp_hit", "mem_load_uops_l3_hit_retired.xsnp_hitm", "mem_load_uops_retired.l3_miss"}
	for _, load := range loads {
		metricName := fmt.Sprintf("Breakdown-%s", load)
		values[metricName] = values[load] / sumOfPreciseLoads(values)
	}
	return 0
}

func lockContention(values map[string]float64) float64 {
	// MEM_LOAD_UOPS_LLC_HIT_RETIRED.XSNP_HITM_PS /MEM_UOPS_RETIRED.LOCK_LOAD_PS
	values["Lock_Contention"] = values["mem_load_uops_l3_hit_retired.xsnp_hitm"] / values["mem_uops_retired.lock_loads"]
	return values["Lock_Contention"]
}

///////////////////////////////////////////////////////////////////////////
////////////////////////// Estimated Load Penalty /////////////////////////
///////////////////////////////////////////////////////////////////////////
func estimatedLoadPenalty(values map[string]float64) float64 {
	ecL1Latency(values)
	ecL2Latency(values)
	ecL3Latency(values)
	ecXSNPLatency(values)
	ecMemLatency(values)
	return 0
}

func ecL1Latency(values map[string]float64) float64 {
	// 5 * MEM_LOAD_UOPS_RETIRED.L1_HIT / CLKS
	values["EC_L1_Latency"] = values["mem_load_uops_retired.l1_hit"] / clks(values)
	return values["EC_L1_Latency"]
}

func ecL2Latency(values map[string]float64) float64 {
	// 12 * MEM_LOAD_UOPS_RETIRED.L2_HIT / CLKS
	values["EC_L2_Latency"] = 12 * values["mem_load_uops_retired.l2_hit"] / clks(values)
	return values["EC_L1_Latency"]
}

func ecL3Latency(values map[string]float64) float64 {
	// 26 * MEM_LOAD_UOPS_RETIRED.L3_HIT / CLKS
	values["EC_L3_Latency"] = 26 * values["mem_load_uops_retired.l3_hit"] / clks(values)
	return values["EC_L1_Latency"]
}

func ecXSNPLatency(values map[string]float64) float64 {
	// 43 * mem_load_uops_l3_hit_retired.xsnp_hit / CLKS
	values["EC_XSNP_Latency"] = 43 * values["mem_load_uops_l3_hit_retired.xsnp_hit"] / clks(values)
	return values["EC_L1_Latency"]
}

func ecMemLatency(values map[string]float64) float64 {
	// 200 * mem_load_uops_retired.l3_miss / CLKS
	values["EC_MEM_Latency"] = 200 * values["mem_load_uops_retired.l3_miss"] / clks(values)
	return values["EC_L1_Latency"]
}

///////////////////////////////////////////////////////////////
////////////////////////// Core Bound /////////////////////////
///////////////////////////////////////////////////////////////
func coreBound(values map[string]float64) float64 {
	// Backend_Bound - Memory_Bound
	values["Core_Bound"] = backendBound(values) - memoryBound(values)
	// values["Core_Bound"] = beBoundAtEXE(values) - memoryBound(values)
	return values["Core_Bound"]
}

func divider(values map[string]float64) float64 {
	// arith.fpu_div_active / CORE_CLKS
	values["Divider"] = values["arith.fpu_div_active"] / clks(values)
	return values["Divider"]
}

func portsUtilization(values map[string]float64) float64 {
	// ( #Backend_Bound_Cycles - RESOURCE_STALLS.SB -  CYCLE_ACTIVITY.STALLS_MEM_ANY ) / CLKS
	values["Ports_Utilization"] = (backendBoundCycles(values) - values["resource_stalls.sb"] - values["cycle_activity.stalls_mem_any"]) / clks(values)
	return values["Ports_Utilization"]
}

func assistsCost(values map[string]float64) float64 {
	// IDQ.MS_CYCLES / CPU_CLK_UNHALTED.THREAD
	values["Assists_Cost"] = values["idq.ms_cycles"] / clks(values)
	return values["Assists_Cost"]
}

func fpAssists(values map[string]float64) float64 {
	// fp_assist.any / inst_retired.any
	values["FP_Assists"] = values["fp_assist.any"] / values["inst_retired.any"]
	return values["FP_Assists"]
}

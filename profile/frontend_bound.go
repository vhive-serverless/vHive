package profile

func frontendBound(values map[string]float64) float64 {
	// IDQ_UOPS_NOT_DELIVERED.CORE / SLOTS
	values["Frontend_Bound"] = values["idq_uops_not_delivered.core"] / slots(values)
	return values["Frontend_Bound"]
}

func fetchLatency(values map[string]float64) float64 {
	// #Pipeline_Width * IDQ_UOPS_NOT_DELIVERED.CYCLES_0_UOPS_DELIV.CORE / SLOTS
	values["Fetch_Latency"] = pipelineWidth * values["idq_uops_not_delivered.cycles_0_uops_deliv.core"] / slots(values)
	return values["Fetch_Latency"]
}

func icacheMisses(values map[string]float64) float64 {
	// ICACHE.IFDATA_STALL / CLKS
	values["ICache_Misses"] = values["icache.ifdata_stall"] / clks(values)
	return values["ICache_Misses"]
}

func itlbMisses(values map[string]float64) float64 {
	// #ITLB_Miss_Cycles / CLKS
	values["ITLB_Misses"] = itlbMissCycles(values) / clks(values)
	return values["ITLB_Misses"]
}

func branchResteers(values map[string]float64) float64 {
	// #BAClear_Cost * ( BR_MISP_RETIRED.ALL_BRANCHES + MACHINE_CLEARS.COUNT + BACLEARS.ANY ) / CLKS
	values["Branch_Resteers"] = 12 * (values["br_misp_retired.all_branches"] + values["machine_clears.count"] + values["baclears.any"]) / clks(values)
	return values["Branch_Resteers"]
}

func dsbSwitches(values map[string]float64) float64 {
	// DSB2MITE_SWITCHES.PENALTY_CYCLES / CLKS
	values["DSB_Switches"] = values["dsb2mite_switches.penalty_cycles"] / clks(values)
	return values["DSB_Switches"]
}

func msSwitches(values map[string]float64) float64 {
	// #MS_Switches_Cost * IDQ.MS_SWITCHES / CLKS
	values["MS_Switches"] = 2 * values["idq.ms_switches"] / clks(values)
	return values["MS_Switches"]
}

func fetchBandwidth(values map[string]float64) float64 {
	// Frontend_Bound - Fetch_Latency
	values["Fetch_Bandwidth"] = frontendBound(values) - fetchLatency(values)
	return values["Fetch_Bandwidth"]
}

func mite(values map[string]float64) float64 {
	// ( IDQ.ALL_MITE_CYCLES_ANY_UOPS - IDQ.ALL_MITE_CYCLES_4_UOPS ) / CORE_CLKS
	values["MITE"] = (values["idq.all_mite_cycles_any_uops"] - values["idq.all_mite_cycles_4_uops"]) / coreClock(values)
	return values["MITE"]
}

func dsb(values map[string]float64) float64 {
	// ( IDQ.ALL_DSB_CYCLES_ANY_UOPS - IDQ.ALL_DSB_CYCLES_4_UOPS ) / CORE_CLKS
	values["DSB"] = (values["idq.all_dsb_cycles_any_uops"] - values["idq.all_dsb_cycles_4_uops"]) / coreClock(values)
	return values["DSB"]
}

func lsd(values map[string]float64) float64 {
	// ( LSD.CYCLES_ACTIVE - LSD.CYCLES_4_UOPS ) / CORE_CLKS
	values["LSD"] = (values["lsd.cycles_active"] - values["lsd.cycles_4_uops"]) / coreClock(values)
	return values["LSD"]
}

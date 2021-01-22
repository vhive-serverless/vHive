package profile

import (
	"encoding/json"
	"errors"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

const (
	pipelineWidth = 4
)

func calculateMetric(name string, values map[string]float64) (float64, error) {
	var (
		funcMap      = getMetricFuncMap()
		f, isPresent = funcMap[name]
	)

	if !isPresent {
		return -1, errors.New("the metric does not exist")
	}

	return f(values), nil
}

func getEvents(metrics map[string][]string, metric string) ([]string, error) {
	events, isPresent := metrics[metric]

	if !isPresent {
		return nil, errors.New("the metric does not exist")
	}

	for i := 0; i < len(events); i++ {
		e := events[i]
		_, isMetric := metrics[e]
		if isMetric {
			subEvents, err := getEvents(metrics, e)
			if err != nil {
				return nil, err
			}
			tmp := append(subEvents, events[i+1:]...)
			events = append(events[:i], tmp...)
		}
	}

	return events, nil
}

///////////////////////////////////////////////////////////////////
////////////////////////// Frontend Bound /////////////////////////
///////////////////////////////////////////////////////////////////
func frontendBound(values map[string]float64) float64 {
	// IDQ_UOPS_NOT_DELIVERED.CORE / SLOTS
	return values["idq_uops_not_delivered.core"] / slots(values)
}

func fetchLatency(values map[string]float64) float64 {
	// #Pipeline_Width * IDQ_UOPS_NOT_DELIVERED.CYCLES_0_UOPS_DELIV.CORE / SLOTS
	return pipelineWidth * values["idq_uops_not_delivered.cycles_0_uops_deliv.core"] / slots(values)
}

func icacheMisses(values map[string]float64) float64 {
	// ICACHE.IFDATA_STALL / CLKS
	return values["icache.ifdata_stall"] / clks(values)
}

func itlbMisses(values map[string]float64) float64 {
	// #ITLB_Miss_Cycles / CLKS
	return itlbMissCycles(values) / clks(values)
}

func branchResteers(values map[string]float64) float64 {
	// #BAClear_Cost * ( BR_MISP_RETIRED.ALL_BRANCHES + MACHINE_CLEARS.COUNT + BACLEARS.ANY ) / CLKS
	return 12 * (values["br_misp_retired.all_branches"] + values["machine_clears.count"] + values["baclears.any"]) / clks(values)
}

func dsbSwitches(values map[string]float64) float64 {
	// DSB2MITE_SWITCHES.PENALTY_CYCLES / CLKS
	return values["dsb2mite_switches.penalty_cycles"] / clks(values)
}

func msSwitches(values map[string]float64) float64 {
	// #MS_Switches_Cost * IDQ.MS_SWITCHES / CLKS
	return 3 * values["idq.ms_switches"] / clks(values)
}

func fetchBandwidth(values map[string]float64) float64 {
	// Frontend_Bound - Fetch_Latency
	return frontendBound(values) - fetchLatency(values)
}

func mite(values map[string]float64) float64 {
	// ( IDQ.ALL_MITE_CYCLES_ANY_UOPS - IDQ.ALL_MITE_CYCLES_4_UOPS ) / CORE_CLKS
	return (values["idq.all_mite_cycles_any_uops"] - values["idq.all_mite_cycles_4_uops"]) / coreClock(values)
}

func dsb(values map[string]float64) float64 {
	// ( IDQ.ALL_DSB_CYCLES_ANY_UOPS - IDQ.ALL_DSB_CYCLES_4_UOPS ) / CORE_CLKS
	return (values["idq.all_dsb_cycles_any_uops"] - values["idq.all_dsb_cycles_4_uops"]) / coreClock(values)
}

func lsd(values map[string]float64) float64 {
	// ( LSD.CYCLES_ACTIVE - LSD.CYCLES_4_UOPS ) / CORE_CLKS
	return (values["lsd.cycles_active"] - values["lsd.cycles_4_uops"]) / coreClock(values)
}

//////////////////////////////////////////////////////////////////
////////////////////////// Bad Speculation ///////////////////////
//////////////////////////////////////////////////////////////////
func badSpeculation(values map[string]float64) float64 {
	// ( UOPS_ISSUED.ANY - UOPS_RETIRED.RETIRE_SLOTS + #Pipeline_Width * #Recovery_Cycles ) / SLOTS
	return (values["uops_issued.any"] - values["uops_retired.retire_slots"] + pipelineWidth*recoveryCycles(values)) / slots(values)
}

func branchMispredicts(values map[string]float64) float64 {
	// Mispred_Clears_Fraction * Bad_Speculation
	return mispredClearsFraction(values) * badSpeculation(values)
}

func machineClears(values map[string]float64) float64 {
	// Bad_Speculation - Branch_Mispredicts
	return badSpeculation(values) - branchMispredicts(values)
}

//////////////////////////////////////////////////////////////////
////////////////////////// Backend Bound /////////////////////////
//////////////////////////////////////////////////////////////////
func backendBound(values map[string]float64) float64 {
	// 1 - ( Frontend_Bound + Bad_Speculation + Retiring )
	return 1 - (frontendBound(values) + badSpeculation(values) + retiring(values))
}

func memoryBound(values map[string]float64) float64 {
	// #Memory_Bound_Fraction * Backend_Bound
	return memoryBoundFraction(values) * backendBound(values)
}

func l1Bound(values map[string]float64) float64 {
	// max ( ( CYCLE_ACTIVITY.STALLS_MEM_ANY - CYCLE_ACTIVITY.STALLS_L1D_MISS ) / CLKS , 0 )
	if val := (values["cycle_activity.stalls_mem_any"] - values["cycle_activity.stalls_l1d_miss"]) / clks(values); val > 0 {
		return val
	}
	return 0
}

func dtlbLoad(values map[string]float64) float64 {
	// ( #Mem_STLB_Hit_Cost * DTLB_LOAD_MISSES.STLB_HIT + DTLB_LOAD_MISSES.WALK_DURATION + 7 * DTLB_LOAD_MISSES.WALK_COMPLETED ) / CLKS
	return (8*values["dtlb_load_misses.stlb_hit"] + values["cpu/dtlb_load_misses.walk_duration,cmask=1/"] + 7*values["dtlb_load_misses.walk_completed"]) / clks(values)
}

func fbFull(values map[string]float64) float64 {
	// Load_Miss_Real_Latency * L1D_PEND_MISS.FB_FULL / CLKS
	return loadMissRealLatency(values) * values["cpu/l1d_pend_miss.fb_full,cmask=1/"] / clks(values)
}

func l2Bound(values map[string]float64) float64 {
	// ( CYCLE_ACTIVITY.STALLS_L1D_MISS - CYCLE_ACTIVITY.STALLS_L2_MISS ) / CLKS
	return (values["cycle_activity.stalls_l1d_miss"] - values["cycle_activity.stalls_l2_miss"]) / clks(values)
}

func l3Bound(values map[string]float64) float64 {
	// Mem_L3_Hit_Fraction * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	return memL3HitFraction(values) * values["cycle_activity.stalls_l2_miss"] / clks(values)
}

func dramBound(values map[string]float64) float64 {
	// ( 1 - Mem_L3_Hit_Fraction ) * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	return (1 - memL3HitFraction(values)) * values["cycle_activity.stalls_l2_miss"] / clks(values)
}

func memBandwidth(values map[string]float64) float64 {
	// #ORO_DRD_BW_Cycles / CLKS
	return oroDRDBWCycles(values) / clks(values)
}

func memLatency(values map[string]float64) float64 {
	// #ORO_DRD_Any_Cycles / CLKS - MEM_Bandwidth
	return oroDRDAnyCycles(values)/clks(values) - memBandwidth(values)
}

func storeBound(values map[string]float64) float64 {
	// RESOURCE_STALLS.SB / CLKS
	return values["resource_stalls.sb"] / clks(values)
}

func coreBound(values map[string]float64) float64 {
	// Backend_Bound - Memory_Bound
	return backendBound(values) - memoryBound(values)
}

func divider(values map[string]float64) float64 {
	// arith.fpu_div_active / CORE_CLKS
	return values["arith.fpu_div_active"] / coreClock(values)
}

func portsUtilization(values map[string]float64) float64 {
	// ( #Backend_Bound_Cycles - RESOURCE_STALLS.SB -  CYCLE_ACTIVITY.STALLS_MEM_ANY ) / CLKS
	return (backendBoundCycles(values) - values["resource_stalls.sb"] - values["cycle_activity.stalls_mem_any"]) / clks(values)
}

/////////////////////////////////////////////////////////////
////////////////////////// Retiring /////////////////////////
/////////////////////////////////////////////////////////////

func retiring(values map[string]float64) float64 {
	// uops_retired.retire_slots / SLOTS
	return values["uops_retired.retire_slots"] / slots(values)
}

/////////////////////////////////////////////////////////
////////////////////////// Info /////////////////////////
/////////////////////////////////////////////////////////
func clks(values map[string]float64) float64 {
	return values["cpu_clk_unhalted.thread"]
}

func slots(values map[string]float64) float64 {
	// #Pipeline_Width * CORE_CLKS
	return pipelineWidth * coreClock(values)
}

func coreClock(values map[string]float64) float64 {
	// cpu_clk_unhalted.thread_any / 2
	return values["cpu_clk_unhalted.thread_any"] / 2
}

func ipc(values map[string]float64) float64 {
	// inst_retired.any / cpu_clk_unhalted.thread
	return values["inst_retired.any"] / values["cpu_clk_unhalted.thread"]
}

func upi(values map[string]float64) float64 {
	// uops_retired.retire_slots / inst_retired.any
	return values["uops_retired.retire_slots"] / values["inst_retired.any"]
}

func ilp(values map[string]float64) float64 {
	// uops_executed.thread / Execute_Cycles
	return values["uops_executed.thread"] / executeCycles(values)
}

func mlp(values map[string]float64) float64 {
	// l1d_pend_miss.pending / l1d_pend_miss.pending_cycles
	return values["l1d_pend_miss.pending"] / values["l1d_pend_miss.pending_cycles"]
}

func loadMissRealLatency(values map[string]float64) float64 {
	// l1d_pend_miss.pending / ( mem_load_uops_retired.l1_miss + mem_load_uops_retired.hit_lfb )
	return values["l1d_pend_miss.pending"] / (values["mem_load_uops_retired.l1_miss"] + values["mem_load_uops_retired.hit_lfb"])
}

func memL3HitFraction(values map[string]float64) float64 {
	// mem_load_uops_retired.l3_hit / ( mem_load_uops_retired.l3_hit + #Mem_L3_Weight * mem_load_uops_retired.l3_miss )
	return values["mem_load_uops_retired.l3_hit"] / (values["mem_load_uops_retired.l3_hit"] + 7*values["mem_load_uops_retired.l3_miss"])
}

func cpuUtilization(values map[string]float64) float64 {
	// cpu_clk_unhalted.ref_tsc / msr/tsc/
	return values["cpu_clk_unhalted.ref_tsc"] / values["msr/tsc/"]
}

func executeCycles(values map[string]float64) float64 {
	// cpu/uops_executed.core,cmask=1/ / 2
	return values["cpu/uops_executed.core,cmask=1/"] / 2
}

func pageWalksUtilization(values map[string]float64) float64 {
	// ( itlb_misses.walk_duration:c1 + dtlb_load_misses.walk_duration:c1 + dtlb_store_misses.walk_duration:c1 + 7 * ( dtlb_store_misses.walk_completed + dtlb_load_misses.walk_completed + itlb_misses.walk_completed ) ) / CORE_CLKS
	return (values["cpu/itlb_misses.walk_duration,cmask=1/"] + values["cpu/dtlb_load_misses.walk_duration,cmask=1/"] + values["cpu/dtlb_store_misses.walk_duration,cmask=1/"] + 7*(values["dtlb_store_misses.walk_completed"]+values["dtlb_load_misses.walk_completed"]+values["itlb_misses.walk_completed"])) / coreClock(values)
}

func smt2TUtilization(values map[string]float64) float64 {
	// 1 - cpu_clk_thread_unhalted.one_thread_active / ( cpu_clk_thread_unhalted.ref_xclk_any / 2 )
	return 1 - values["cpu_clk_thread_unhalted.one_thread_active"]/(values["cpu_clk_thread_unhalted.ref_xclk_any"]/2)
}

func recoveryCycles(values map[string]float64) float64 {
	// int_misc.recovery_cycles_any / 2
	return values["int_misc.recovery_cycles_any"] / 2
}

func mispredClearsFraction(values map[string]float64) float64 {
	// br_misp_retired.all_branches / ( br_misp_retired.all_branches + machine_clears.count )
	return values["br_misp_retired.all_branches"] / (values["br_misp_retired.all_branches"] + values["machine_clears.count"])
}

func memoryBoundFraction(values map[string]float64) float64 {
	// ( cycle_activity.stalls_mem_any + resource_stalls.sb ) / #Backend_Bound_Cycles
	return (values["cycle_activity.stalls_mem_any"] + values["resource_stalls.sb"]) / backendBoundCycles(values)
}

func backendBoundCycles(values map[string]float64) float64 {
	// ( cycle_activity.stalls_total + UOPS_EXECUTED.CYCLES_GE_1_UOP_EXEC - #Few_Uops_Executed_Threshold - #Frontend_RS_Empty_Cycles + RESOURCE_STALLS.SB )
	return values["cycle_activity.stalls_total"] + values["uops_executed.cycles_ge_1_uop_exec"] - fewUopsExecutedThreshold(values) - frontendRSEmptyCycles(values) + values["resource_stalls.sb"]
}

func fewUopsExecutedThreshold(values map[string]float64) float64 {
	// uops_executed.cycles_ge_3_uops_exec if ( IPC > 1.8 ) else uops_executed.cycles_ge_2_uops_exec
	if ipc(values) > 1.8 {
		return values["uops_executed.cycles_ge_3_uops_exec"]
	}
	return values["uops_executed.cycles_ge_2_uops_exec"]
}

func frontendRSEmptyCycles(values map[string]float64) float64 {
	// rs_events.empty_cycles if ( Fetch_Latency > 0.1 ) else 0
	if fetchLatency(values) > 0.1 {
		return values["rs_events.empty_cycles"]
	}
	return 0
}

func oroDRDAnyCycles(values map[string]float64) float64 {
	// min( CPU_CLK_UNHALTED.THREAD , OFFCORE_REQUESTS_OUTSTANDING.CYCLES_WITH_DATA_RD )
	if values["cpu_clk_unhalted.thread"] < values["offcore_requests_outstanding.cycles_with_data_rd"] {
		return values["cpu_clk_unhalted.thread"]
	}

	return values["offcore_requests_outstanding.cycles_with_data_rd"]
}

func oroDRDBWCycles(values map[string]float64) float64 {
	// min( CPU_CLK_UNHALTED.THREAD , OFFCORE_REQUESTS_OUTSTANDING.ALL_DATA_RD:c4 )
	if values["cpu_clk_unhalted.thread"] < values["cpu/offcore_requests_outstanding.all_data_rd,cmask=4/"] {
		return values["cpu_clk_unhalted.thread"]
	}

	return values["cpu/offcore_requests_outstanding.all_data_rd,cmask=4/"]
}

func itlbMissCycles(values map[string]float64) float64 {
	// 14 * ITLB_MISSES.STLB_HIT + cpu/itlb_misses.walk_duration,cmask=1/ + 7 * ITLB_MISSES.WALK_COMPLETED
	return 14*values["itlb_misses.stlb_hit"] + values["cpu/itlb_misses.walk_duration,cmask=1/"] + 7*values["itlb_misses.walk_completed"]
}

///////////////////////////////////////////////////////////////////////////////
////////////////////////// Auxialiary functions below /////////////////////////
///////////////////////////////////////////////////////////////////////////////
type metrics struct {
	Metrics []metric
}

type metric struct {
	Name         string
	Events       []string
	ChildMetrics []metric
}

func getMetricEvents(metric metric) map[string][]string {
	result := make(map[string][]string)

	if len(metric.Events) > 0 {
		result[metric.Name] = metric.Events
	}

	for _, child := range metric.ChildMetrics {
		childMap := getMetricEvents(child)
		for name, events := range childMap {
			result[name] = events
		}
	}

	return result
}

func getMetrics() map[string][]string {
	metrics := readMetricsJSON()
	result := make(map[string][]string)

	for _, metric := range metrics.Metrics {
		if len(metric.Events) > 0 {
			metrics := getMetricEvents(metric)
			for name, events := range metrics {
				result[name] = events
			}
		}
	}

	return result
}

func readMetricsJSON() metrics {
	data, err := ioutil.ReadFile("profile/metrics.json")
	if err != nil {
		log.Fatal("Cannot read JSON file")
	}
	var ms metrics
	err = json.Unmarshal(data, &ms)
	if err != nil {
		log.Fatal("Cannot parse JSON file")
	}

	return ms
}

func getMetricFuncMap() map[string]func(map[string]float64) float64 {
	return map[string]func(map[string]float64) float64{
		"CLKS":                        clks,
		"CORE_CLKS":                   coreClock,
		"IPC":                         ipc,
		"UPI":                         upi,
		"ILP":                         ilp,
		"MLP":                         mlp,
		"SLOTS":                       slots,
		"Load_Miss_Real_Latency":      loadMissRealLatency,
		"Mem_L3_Hit_Fraction":         memL3HitFraction,
		"CPU_Utilization":             cpuUtilization,
		"Execute_Cycles":              executeCycles,
		"Page_Walks_Utilization":      pageWalksUtilization,
		"SMT_2T_Utilization":          smt2TUtilization,
		"Recovery_Cycles":             recoveryCycles,
		"Mispred_Clears_Fraction":     mispredClearsFraction,
		"Memory_Bound_Fraction":       memoryBoundFraction,
		"Backend_Bound_Cycles":        backendBoundCycles,
		"Few_Uops_Executed_Threshold": fewUopsExecutedThreshold,
		"Frontend_RS_Empty_Cycles":    frontendRSEmptyCycles,
		"ORO_DRD_Any_Cycles":          oroDRDAnyCycles,
		"ORO_DRD_BW_Cycles":           oroDRDBWCycles,
		"ITLB_Miss_Cycles":            itlbMissCycles,

		// Frontend Bound
		"Frontend_Bound":  frontendBound,
		"Fetch_Latency":   fetchLatency,
		"ICache_Misses":   icacheMisses,
		"ITLB_Misses":     itlbMisses,
		"Branch_Resteers": branchResteers,
		"DSB_Switches":    dsbSwitches,
		"MS_Switches":     msSwitches,
		"Fetch_Bandwidth": fetchBandwidth,
		"MITE":            mite,
		"DSB":             dsb,
		"LSD":             lsd,

		// Bad_Speculation
		"Bad_Speculation":    badSpeculation,
		"Branch_Mispredicts": branchMispredicts,
		"Machine_Clears":     machineClears,

		// Backend Bound
		"Backend_Bound":     backendBound,
		"Memory_Bound":      memoryBound,
		"L1_Bound":          l1Bound,
		"DTLB_Load":         dtlbLoad,
		"FB_Full":           fbFull,
		"L2_Bound":          l2Bound,
		"L3_Bound":          l3Bound,
		"DRAM_Bound":        dramBound,
		"Store_Bound":       storeBound,
		"MEM_Bandwidth":     memBandwidth,
		"MEM_Latency":       memLatency,
		"Core_Bound":        coreBound,
		"Divider":           divider,
		"Ports_Utilization": portsUtilization,

		// Retiring
		"Retiring": retiring,
	}
}

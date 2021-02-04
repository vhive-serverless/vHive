package profile

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

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

/////////////////////////////////////////////////////////
////////////////////////// Info /////////////////////////
/////////////////////////////////////////////////////////
func clks(values map[string]float64) float64 {
	return values["cpu_clk_unhalted.thread"]
}

func slots(values map[string]float64) float64 {
	// values["SLOTS"] = pipelineWidth * coreClock(values)
	values["SLOTS"] = pipelineWidth * clks(values)
	return values["SLOTS"]
}

func coreClock(values map[string]float64) float64 {
	// cpu_clk_unhalted.thread_any / 2
	values["CORE_CLKS"] = values["cpu_clk_unhalted.thread_any"] / 2
	return values["CORE_CLKS"]
}

func ipc(values map[string]float64) float64 {
	// inst_retired.any / cpu_clk_unhalted.thread
	values["IPC"] = values["inst_retired.any"] / values["cpu_clk_unhalted.thread"]
	return values["IPC"]
}

func upi(values map[string]float64) float64 {
	// uops_retired.retire_slots / inst_retired.any
	values["UPI"] = values["uops_retired.retire_slots"] / values["inst_retired.any"]
	return values["UPI"]
}

func ilp(values map[string]float64) float64 {
	// uops_executed.thread / Execute_Cycles
	values["ILP"] = values["uops_executed.thread"] / executeCycles(values)
	return values["ILP"]
}

func mlp(values map[string]float64) float64 {
	// l1d_pend_miss.pending / l1d_pend_miss.pending_cycles
	values["MLP"] = values["l1d_pend_miss.pending"] / values["l1d_pend_miss.pending_cycles"]
	return values["MLP"]
}

func loadMissRealLatency(values map[string]float64) float64 {
	// l1d_pend_miss.pending / ( mem_load_uops_retired.l1_miss + mem_load_uops_retired.hit_lfb )
	values["Load_Miss_Real_Latency"] = values["l1d_pend_miss.pending"] / (values["mem_load_uops_retired.l1_miss"] + values["mem_load_uops_retired.hit_lfb"])
	return values["Load_Miss_Real_Latency"]
}

func memL3HitFraction(values map[string]float64) float64 {
	// mem_load_uops_retired.l3_hit / ( mem_load_uops_retired.l3_hit + #Mem_L3_Weight * mem_load_uops_retired.l3_miss )
	values["Mem_L3_Hit_Fraction"] = values["mem_load_uops_retired.l3_hit"] / (values["mem_load_uops_retired.l3_hit"] + 7*values["mem_load_uops_retired.l3_miss"])
	return values["Mem_L3_Hit_Fraction"]
}

func cpuUtilization(values map[string]float64) float64 {
	// cpu_clk_unhalted.ref_tsc / msr/tsc/
	values["CPU_Utilization"] = values["cpu_clk_unhalted.ref_tsc"] / values["msr/tsc/"]
	return values["CPU_Utilization"]
}

func executeCycles(values map[string]float64) float64 {
	// cpu/uops_executed.core,cmask=1/ / 2
	values["Execute_Cycles"] = values["cpu/uops_executed.core,cmask=1/"] / 2
	return values["Execute_Cycles"]
}

func pageWalksUtilization(values map[string]float64) float64 {
	// ( itlb_misses.walk_duration:c1 + dtlb_load_misses.walk_duration:c1 + dtlb_store_misses.walk_duration:c1 + 7 * ( dtlb_store_misses.walk_completed + dtlb_load_misses.walk_completed + itlb_misses.walk_completed ) ) / CORE_CLKS
	values["Page_Walks_Utilization"] = (values["cpu/itlb_misses.walk_duration,cmask=1/"] + values["cpu/dtlb_load_misses.walk_duration,cmask=1/"] + values["cpu/dtlb_store_misses.walk_duration,cmask=1/"] + 7*(values["dtlb_store_misses.walk_completed"]+values["dtlb_load_misses.walk_completed"]+values["itlb_misses.walk_completed"])) / coreClock(values)
	return values["Page_Walks_Utilization"]
}

func smt2TUtilization(values map[string]float64) float64 {
	// 1 - cpu_clk_thread_unhalted.one_thread_active / ( cpu_clk_thread_unhalted.ref_xclk_any / 2 )
	values["SMT_2T_Utilization"] = 1 - values["cpu_clk_thread_unhalted.one_thread_active"]/(values["cpu_clk_thread_unhalted.ref_xclk_any"]/2)
	return values["SMT_2T_Utilization"]
}

func recoveryCycles(values map[string]float64) float64 {
	// int_misc.recovery_cycles_any / 2
	// values["Recovery_Cycles"] = values["int_misc.recovery_cycles_any"] / 2
	// return values["Recovery_Cycles"]
	return values["int_misc.recovery_cycles"]
}

func mispredClearsFraction(values map[string]float64) float64 {
	// br_misp_retired.all_branches / ( br_misp_retired.all_branches + machine_clears.count )
	values["Mispred_Clears_Fraction"] = values["br_misp_retired.all_branches"] / (values["br_misp_retired.all_branches"] + values["machine_clears.count"])
	return values["Mispred_Clears_Fraction"]
}

func memoryBoundFraction(values map[string]float64) float64 {
	// ( cycle_activity.stalls_mem_any + resource_stalls.sb ) / #Backend_Bound_Cycles
	values["Memory_Bound_Fraction"] = (values["cycle_activity.stalls_mem_any"] + values["resource_stalls.sb"]) / backendBoundCycles(values)
	return values["Memory_Bound_Fraction"]
}

func backendBoundCycles(values map[string]float64) float64 {
	// ( cycle_activity.stalls_total + UOPS_EXECUTED.CYCLES_GE_1_UOP_EXEC - #Few_Uops_Executed_Threshold - #Frontend_RS_Empty_Cycles + RESOURCE_STALLS.SB )
	values["Backend_Bound_Cycles"] = values["cycle_activity.stalls_total"] + values["uops_executed.cycles_ge_1_uop_exec"] - fewUopsExecutedThreshold(values) - frontendRSEmptyCycles(values) + values["resource_stalls.sb"]
	return values["Backend_Bound_Cycles"]
}

func fewUopsExecutedThreshold(values map[string]float64) float64 {
	// uops_executed.cycles_ge_3_uops_exec if ( IPC > 1.8 ) else uops_executed.cycles_ge_2_uops_exec
	if ipc(values) > 1.8 {
		values["Few_Uops_Executed_Threshold"] = values["uops_executed.cycles_ge_3_uops_exec"]
	} else {
		values["Few_Uops_Executed_Threshold"] = values["uops_executed.cycles_ge_2_uops_exec"]
	}
	return values["Few_Uops_Executed_Threshold"]
}

func frontendRSEmptyCycles(values map[string]float64) float64 {
	// rs_events.empty_cycles if ( Fetch_Latency > 0.1 ) else 0
	if fetchLatency(values) > 0.1 {
		values["Frontend_RS_Empty_Cycles"] = values["rs_events.empty_cycles"]
	} else {
		values["Frontend_RS_Empty_Cycles"] = 0
	}
	return values["Frontend_RS_Empty_Cycles"]
}

func oroDRDAnyCycles(values map[string]float64) float64 {
	// min( CPU_CLK_UNHALTED.THREAD , OFFCORE_REQUESTS_OUTSTANDING.CYCLES_WITH_DATA_RD )
	if values["cpu_clk_unhalted.thread"] < values["offcore_requests_outstanding.cycles_with_data_rd"] {
		values["ORO_DRD_Any_Cycles"] = values["cpu_clk_unhalted.thread"]
	} else {
		values["ORO_DRD_Any_Cycles"] = values["offcore_requests_outstanding.cycles_with_data_rd"]
	}
	return values["ORO_DRD_Any_Cycles"]
}

func oroDRDBWCycles(values map[string]float64) float64 {
	// min( CPU_CLK_UNHALTED.THREAD , OFFCORE_REQUESTS_OUTSTANDING.ALL_DATA_RD:c4 )
	if values["cpu_clk_unhalted.thread"] < values["cpu/offcore_requests_outstanding.all_data_rd,cmask=4/"] {
		values["ORO_DRD_BW_Cycles"] = values["cpu_clk_unhalted.thread"]
	} else {
		values["ORO_DRD_BW_Cycles"] = values["cpu/offcore_requests_outstanding.all_data_rd,cmask=4/"]
	}
	return values["ORO_DRD_BW_Cycles"]
}

func itlbMissCycles(values map[string]float64) float64 {
	// 14 * ITLB_MISSES.STLB_HIT + cpu/itlb_misses.walk_duration,cmask=1/ + 7 * ITLB_MISSES.WALK_COMPLETED
	values["ITLB_Miss_Cycles"] = 14*values["itlb_misses.stlb_hit"] + values["cpu/itlb_misses.walk_duration,cmask=1/"] + 7*values["itlb_misses.walk_completed"]
	return values["ITLB_Miss_Cycles"]
}

func resourceStallsFraction(values map[string]float64) float64 {
	// resource_stalls.any / CPU_CLK_UNHALTED.THREAD
	values["Resource_Stalls_Fraction"] = values["resource_stalls.any"] / clks(values)
	return values["Resource_Stalls_Fraction"]
}

func sumOfPreciseLoads(values map[string]float64) float64 {
	// MEM_LOAD_UOPS_RETIRED.HIT_LFB_PS +MEM_LOAD_UOPS_RETIRED.L1_HIT_PS + MEM_LOAD_UOPS_RETIRED.L2_HIT_PS +MEM_LOAD_UOPS_RETIRED.LLC_HIT_PS +MEM_LOAD_UOPS_LLC_HIT_RETIRED.XSNP_MISS +MEM_LOAD_UOPS_LLC_HIT_RETIRED.XSNP_HIT_PS +MEM_LOAD_UOPS_LLC_HIT_RETIRED.XSNP_HITM_PS +mem_load_uops_retired.l3_miss
	values["SumOf_PRECISE_LOADS"] = values["mem_uops_retired.all_loads"]
	// values["SumOf_PRECISE_LOADS"] = values["mem_load_uops_retired.hit_lfb"] + values["mem_load_uops_retired.l1_hit"] + values["mem_load_uops_retired.l2_hit"] + values["mem_load_uops_retired.l3_hit"] + values["mem_load_uops_l3_hit_retired.xsnp_miss"] + values["mem_load_uops_l3_hit_retired.xsnp_hit"] + values["mem_load_uops_l3_hit_retired.xsnp_hitm"] + values["mem_load_uops_retired.l3_miss"]
	return values["SumOf_PRECISE_LOADS"]
}

func retireFraction(values map[string]float64) float64 {
	// #Retired_Slots / UOPS_ISSUED.ANY
	values["Retire_Fraction"] = values["uops_retired.retire_slots"] / values["uops_issued.any"]
	return values["Retire_Fraction"]
}

func fpArithScalar(values map[string]float64) float64 {
	// FP_ARITH_INST_RETIRED.SCALAR_SINGLE + FP_ARITH_INST_RETIRED.SCALAR_DOUBLE
	values["FP_Arith_Scalar"] = values["fp_arith_inst_retired.scalar_single"] + values["fp_arith_inst_retired.scalar_double"]
	return values["FP_Arith_Scalar"]
}

func fpArithVector(values map[string]float64) float64 {
	// FP_ARITH_INST_RETIRED.128B_PACKED_DOUBLE + FP_ARITH_INST_RETIRED.128B_PACKED_SINGLE + FP_ARITH_INST_RETIRED.256B_PACKED_DOUBLE + FP_ARITH_INST_RETIRED.256B_PACKED_SINGLE
	values["FP_Arith_Vector"] = values["fp_arith_inst_retired.128b_packed_double"] + values["fp_arith_inst_retired.128b_packed_single"] + values["fp_arith_inst_retired.256b_packed_double"] + values["fp_arith_inst_retired.256b_packed_single"]
	return values["FP_Arith_Vector"]
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
	path, err := os.Getwd()
	if err != nil {
		log.Fatal("Cannot retrieve current path.")
	}
	if path[len(path)-7:] != "profile" {
		path = filepath.Join(path, "profile")
	}
	path = filepath.Join(path, "metrics.json")
	data, err := ioutil.ReadFile(path)
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
		"Resource_Stalls_Fraction":    resourceStallsFraction,
		"SumOf_PRECISE_LOADS":         sumOfPreciseLoads,
		"Retire_Fraction":             retireFraction,
		"FP_Arith_Scalar":             fpArithScalar,
		"FP_Arith_Vector":             fpArithVector,

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
		"Backend_Bound":          backendBound,
		"BE_Bound_at_EXE":        beBoundAtEXE,
		"Memory_Bound":           memoryBound,
		"L1_Bound":               l1Bound,
		"DTLB_Load":              dtlbLoad,
		"FB_Full":                fbFull,
		"L2_Bound":               l2Bound,
		"L3_Bound":               l3Bound,
		"DRAM_Bound":             dramBound,
		"Uncore_Bound":           uncoreBound,
		"Store_Bound":            storeBound,
		"ROB_Bound":              robBound,
		"Precise_Load_Breakdown": preciseLoadBreakdown,
		"MEM_Bandwidth":          memBandwidth,
		"MEM_Latency":            memLatency,
		"Estimated_Load_Penalty": estimatedLoadPenalty,
		"EC_L1_Latency":          ecL1Latency,
		"EC_L2_Latency":          ecL2Latency,
		"EC_L3_Latency":          ecL3Latency,
		"EC_XSNP_Latency":        ecXSNPLatency,
		"EC_MEM_Latency":         ecMemLatency,
		"Lock_Contention":        lockContention,
		"Core_Bound":             coreBound,
		"Divider":                divider,
		"Ports_Utilization":      portsUtilization,
		"Assists_Cost":           assistsCost,
		"FP_Assists":             fpAssists,

		// Retiring
		"Retiring":            retiring,
		"FP_Scalar":           fpScalar,
		"FP_Vector":           fpVector,
		"Microcode_Sequencer": microcodeSequencer,
	}
}

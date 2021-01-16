package profile

import (
	"errors"
	"reflect"
	"unicode"
)

const (
	pipelineWidth  = 4
	memL3Weight    = 7
	memSTLBHitCost = 8
)

func calculateMetric(name string, params ...float64) (float64, error) {
	var (
		funcMap               = getMetricFuncMap()
		fInterface, isPresent = funcMap[name]
	)

	if !isPresent {
		return -1, errors.New("the metric does not exist")
	}

	f := reflect.ValueOf(fInterface)

	if len(params) != f.Type().NumIn() {
		return -1, errors.New("the number of params does not match with input function")
	}

	in := make([]reflect.Value, len(params))
	for i, param := range params {
		in[i] = reflect.ValueOf(param)
	}

	return f.Call(in)[0].Interface().(float64), nil
}

func getEvents(metric string) ([]string, error) {
	var (
		metricEventMap    = getMetricEventsMap()
		events, isPresent = metricEventMap[metric]
	)

	if !isPresent {
		return nil, errors.New("the metric does not exist")
	}

	for i, e := range events {
		_, isMetric := metricEventMap[e]
		if isUpper(e) && isMetric {
			subEvents, err := getEvents(e)
			if err != nil {
				return nil, err
			}
			tmp := append(events[:i], subEvents...)
			events = append(tmp, events[i+1:]...)
		}
	}

	return events, nil
}

///////////////////////////////////////////////////////////////////
////////////////////////// Frontend Bound /////////////////////////
///////////////////////////////////////////////////////////////////
func frontendBound(iundCore, clksAny float64) float64 {
	// IDQ_UOPS_NOT_DELIVERED.CORE / SLOTS
	return iundCore / slots(clksAny)
}

func fetchLatency(idqNotDelivered, clksAny float64) float64 {
	// #Pipeline_Width * IDQ_UOPS_NOT_DELIVERED.CYCLES_0_UOPS_DELIV.CORE / SLOTS
	return pipelineWidth * idqNotDelivered / slots(clksAny)
}

func icacheMisses(icacheIfdataStall, clks float64) float64 {
	// ICACHE.IFDATA_STALL / CLKS
	return icacheIfdataStall / clks
}

//////////////////////////////////////////////////////////////////
////////////////////////// Backend Bound /////////////////////////
//////////////////////////////////////////////////////////////////
func l1Bound(caStallsMemAny, caStallsL1dMiss, clks float64) float64 {
	// max ( ( CYCLE_ACTIVITY.STALLS_MEM_ANY - CYCLE_ACTIVITY.STALLS_L1D_MISS ) / CLKS , 0 )
	val := (caStallsMemAny - caStallsL1dMiss) / clks
	if val < 0 {
		return 0
	}
	return val
}

func dtlbLoad(dlmStlbHit, dlmWalkDur, dlmWalkCompleted, clks float64) float64 {
	// ( #Mem_STLB_Hit_Cost * DTLB_LOAD_MISSES.STLB_HIT + DTLB_LOAD_MISSES.WALK_DURATION + 7 * DTLB_LOAD_MISSES.WALK_COMPLETED ) / CLKS
	return (float64(memSTLBHitCost)*dlmStlbHit + dlmWalkDur + 7*dlmWalkCompleted) / clks
}

func fbFull(l1pmPending, mlurL1Miss, mlurHitlfb, lpmFbFulls, clks float64) float64 {
	// Load_Miss_Real_Latency * L1D_PEND_MISS.FB_FULL / CLKS
	return loadMissRealLatency(l1pmPending, mlurL1Miss, mlurHitlfb) * lpmFbFulls / clks
}

func l2Bound(caStallsL1dMiss, caStallsL2Miss, clks float64) float64 {
	// ( CYCLE_ACTIVITY.STALLS_L1D_MISS - CYCLE_ACTIVITY.STALLS_L2_MISS ) / CLKS
	return (caStallsL1dMiss - caStallsL2Miss) / clks
}

func l3Bound(mlurL3Hit, mlurL3Miss, caStallsL2Miss, clks float64) float64 {
	// Mem_L3_Hit_Fraction * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	return memL3HitFraction(mlurL3Hit, mlurL3Miss) * caStallsL2Miss / clks
}

func dramBound(mlurL3Hit, mlurL3Miss, caStallsL2Miss, clks float64) float64 {
	// ( 1 - Mem_L3_Hit_Fraction ) * CYCLE_ACTIVITY.STALLS_L2_MISS / CLKS
	return (1 - memL3HitFraction(mlurL3Hit, mlurL3Miss)) * caStallsL2Miss / clks
}

/////////////////////////////////////////////////////////
////////////////////////// Info /////////////////////////
/////////////////////////////////////////////////////////
func slots(clksAny float64) float64 {
	// #Pipeline_Width * CORE_CLKS
	return pipelineWidth * coreClock(clksAny)
}

func coreClock(clksAny float64) float64 {
	// cpu_clk_unhalted.thread_any / 2
	return clksAny / 2
}

func ipc(ins, clks float64) float64 {
	// inst_retired.any / cpu_clk_unhalted.thread
	return ins / clks
}

func loadMissRealLatency(l1pmPending, mlurL1Miss, mlurHitlfb float64) float64 {
	// l1d_pend_miss.pending / ( mem_load_uops_retired.l1_miss + mem_load_uops_retired.hit_lfb )
	return l1pmPending / (mlurL1Miss + mlurHitlfb)
}

func memL3HitFraction(mlurL3Hit, mlurL3Miss float64) float64 {
	// MEM_LOAD_UOPS_RETIRED.L3_HIT_PS / ( MEM_LOAD_UOPS_RETIRED.L3_HIT_PS + #Mem_L3_Weight * MEM_LOAD_UOPS_RETIRED.L3_MISS_PS )
	return mlurL3Hit / (mlurL3Hit + float64(memL3Weight)*mlurL3Miss)
}

///////////////////////////////////////////////////////////////////////////////
////////////////////////// Auxialiary functions below /////////////////////////
///////////////////////////////////////////////////////////////////////////////
func getMetricEventsMap() map[string][]string {
	return map[string][]string{
		"CLKS":                   []string{"cpu_clk_unhalted.thread"},
		"CORE_CLKS":              []string{"cpu_clk_unhalted.thread_any"},
		"IPC":                    []string{"inst_retired.any", "cpu_clk_unhalted.thread"},
		"SLOTS":                  []string{"CORE_CLKS"},
		"Load_Miss_Real_Latency": []string{"l1d_pend_miss.pending", "mem_load_uops_retired.l1_miss", "mem_load_uops_retired.hit_lfb"},
		"Mem_L3_Hit_Fraction":    []string{"mem_load_uops_retired.l3_hit", "mem_load_uops_retired.l3_hit", "mem_load_uops_retired.l3_miss"},

		// Frontend Bound
		"Frontend_Bound": []string{"idq_uops_not_delivered.core", "SLOTS"},
		"Fetch_Latency":  []string{"idq_uops_not_delivered.cycles_0_uops_deliv.core", "SLOTS"},
		"ICache_Misses":  []string{"icache.ifdata_stall", "CLKS"},

		// Backend Bound
		"L1_Bound":   []string{"cycle_activity.stalls_mem_any", "cycle_activity.stalls_l1d_miss", "CLKS"},
		"DTLB_Load":  []string{"dtlb_load_misses.stlb_hit", "dtlb_load_misses.walk_duration", "dtlb_load_misses.walk_completed", "CLKS"},
		"FB_Full":    []string{"Load_Miss_Real_Latency", "l1d_pend_miss.fb_full", "CLKS"},
		"L2_Bound":   []string{"cycle_activity.stalls_l1d_miss", "cycle_activity.stalls_l2_miss", "CLKS"},
		"L3_Bound":   []string{"Mem_L3_Hit_Fraction", "cycle_activity.stalls_l2_miss", "CLKS"},
		"DRAM_Bound": []string{"Mem_L3_Hit_Fraction", "cycle_activity.stalls_l2_miss", "CLKS"},
	}
}

func getMetricFuncMap() map[string]interface{} {
	return map[string]interface{}{
		"CORE_CLKS":              coreClock,
		"IPC":                    ipc,
		"SLOTS":                  slots,
		"Load_Miss_Real_Latency": loadMissRealLatency,
		"Mem_L3_Hit_Fraction":    memL3HitFraction,

		// Frontend Bound
		"Frontend_Bound": frontendBound,
		"Fetch_Latency":  fetchLatency,
		"ICache_Misses":  icacheMisses,

		// Backend Bound
		"L1_Bound":   l1Bound,
		"DTLB_Load":  dtlbLoad,
		"FB_Full":    fbFull,
		"L2_Bound":   l2Bound,
		"L3_Bound":   l3Bound,
		"DRAM_Bound": dramBound,
	}
}

func isUpper(s string) bool {
	for _, r := range s {
		if !unicode.IsUpper(r) && unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

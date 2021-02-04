package profile

func retiring(values map[string]float64) float64 {
	// uops_retired.retire_slots / SLOTS
	values["Retiring"] = values["uops_retired.retire_slots"] / slots(values)
	return values["Retiring"]
}

func fpScalar(values map[string]float64) float64 {
	// #FP_Arith_Scalar / UOPS_RETIRED.RETIRE_SLOTS
	values["FP_Scalar"] = values["FP_Arith_Scalar"] / values["uops_retired.retire_slots"]
	return values["FP_Scalar"]
}

func fpVector(values map[string]float64) float64 {
	// #FP_Arith_Vector / UOPS_RETIRED.RETIRE_SLOTS
	values["FP_Vector"] = values["FP_Arith_Vector"] / values["uops_retired.retire_slots"]
	return values["FP_Vector"]
}

func microcodeSequencer(values map[string]float64) float64 {
	// #Retire_Fraction * IDQ.MS_UOPS / SLOTS
	values["Microcode_Sequencer"] = retireFraction(values) * values["idq.ms_cycles"] / slots(values)
	return values["Microcode_Sequencer"]
}

func getRetiringFuncMap() map[string]func(map[string]float64) float64 {
	return nil
}

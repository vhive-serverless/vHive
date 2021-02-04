package profile

func badSpeculation(values map[string]float64) float64 {
	// ( UOPS_ISSUED.ANY - UOPS_RETIRED.RETIRE_SLOTS + #Pipeline_Width * #Recovery_Cycles ) / SLOTS
	values["Bad_Speculation"] = (values["uops_issued.any"] - values["uops_retired.retire_slots"] + pipelineWidth*recoveryCycles(values)) / slots(values)
	return values["Bad_Speculation"]
}

func branchMispredicts(values map[string]float64) float64 {
	// Mispred_Clears_Fraction * Bad_Speculation
	values["Branch_Mispredicts"] = mispredClearsFraction(values) * badSpeculation(values)
	return values["Branch_Mispredicts"]
}

func machineClears(values map[string]float64) float64 {
	// Bad_Speculation - Branch_Mispredicts
	values["Machine_Clears"] = badSpeculation(values) - branchMispredicts(values)
	return values["Machine_Clears"]
}

package validators

type NextTurnValidators struct {
	WorkingHeight uint64
	Validators    [][]byte
	TotalCount    int
}

func NewNextTurnValidators(workingHeight uint64, validators [][]byte, totalCount int) *NextTurnValidators {
	return &NextTurnValidators{
		WorkingHeight: workingHeight,
		Validators:    validators,
		TotalCount:    totalCount,
	}
}

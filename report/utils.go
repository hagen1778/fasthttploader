package report

import (
	"strconv"
	"strings"
)

func uint64SliceToString(sl []uint64) string {
	str := []string{}
	for _, v := range sl {
		str = append(str, strconv.FormatInt(int64(v), 10))
	}
	return strings.Join(str[:], ",")
}

func float64SliceToString(sl []float64) string {
	str := []string{}
	for _, v := range sl {
		str = append(str, strconv.FormatFloat(v, 'f', 8, 64))
	}
	return strings.Join(str[:], ",")
}

// Rate calculate difference between current and previous value
func rate(sl []uint64, step float64) []float64 {
	result := make([]float64, len(sl))
	if len(sl) < 2 {
		return result
	}

	for i := 1; i < len(sl); i++ {
		curValue := float64(sl[i-1])
		lastValue := float64(sl[i])

		// avoid of unnatural gaps when counter metrics flushed
		if lastValue < curValue {
			lastValue = curValue
		}
		result[i] = (lastValue - curValue) / step
	}

	return result
}

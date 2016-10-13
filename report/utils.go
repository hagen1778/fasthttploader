package report

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
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

// rate calculate difference between current and previous value
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

func mustPwd() string {
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return pwd
}

// OpenBrowser opens given url by default browser
func OpenBrowser(fileName string) error {
	var err error

	url := fmt.Sprintf("file:///%s/%s", mustPwd(), fileName)
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	return err
}

// PrintOpenBrowser prints command to open given url by default browser from console
func PrintOpenBrowser(fileName string) (command string, err error) {
	url := fmt.Sprintf("file:///%s/%s", mustPwd(), fileName)
	switch runtime.GOOS {
	case "linux":
		command = fmt.Sprintf("xdg-open %s", url)
	case "windows":
		command = fmt.Sprintf("start %s", url)
	case "darwin":
		command = fmt.Sprintf("open %s", url)
	default:
		err = fmt.Errorf("unsupported platform")
	}

	return
}

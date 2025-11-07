package utils

import (
	"go-audio-streamer/constants"
	"math"
	"slices"

	"github.com/argusdusty/gofft"
	"github.com/sirupsen/logrus"
)

func SetLog(level logrus.Level) {
	Formatter := new(logrus.TextFormatter)
	Formatter.TimestampFormat = "Jan _2 15:04:05.000000000"
	Formatter.FullTimestamp = true
	Formatter.ForceColors = true
	logrus.SetReportCaller(true)
	// logrus.AddHook(filename.NewHook(level)) // Print filename + line at every log
	logrus.SetFormatter(Formatter)
	logrus.SetLevel(level)
}

// noiseGate reduces background noise by zero-ing frequency bins below a threshold.
// It works on a single 20 ms frame (960 samples @48 kHz).
func NoiseGateFFT(pcm []int16, thresholdDB int) []int16 {
	// const frameSize = constants.FrameSize // 960
	out := make([]int16, 1024)
	copy(out, pcm)

	// 1. Convert to float32 [-1,1]
	inF := make([]float64, 1024) // PAD to the next power of 2
	for i := range out {
		inF[i] = float64(out[i]) / 32768.0
	}

	tmp := gofft.Float64ToComplex128Array(inF)
	// 2. Hann window + FFT (real-only, use go-dsp/fftw or a simple Go impl)
	if err := gofft.FFT(tmp); err != nil {
		logrus.Fatal(err)
	}

	inF = gofft.Complex128ToFloat64Array(tmp)
	// 3. Convert threshold to linear gain
	threshLin := math.Pow10(thresholdDB / 20)

	// 4. Gate
	for i := range inF {
		if inF[i] < threshLin {
			inF[i] = 0
		}
	}

	tmp = gofft.Float64ToComplex128Array(inF)
	// 5. IFFT → PCM
	if err := gofft.IFFT(tmp); err != nil {
		logrus.Fatal(err)
	}

	tmp = tmp[:constants.FrameSize]

	for i := range tmp {
		v := int16(math.Max(-32768, math.Min(32767, real(tmp[i])*32768)))
		out[i] = v
	}

	return out
}

func NaiveNoiseGate(pcm []int16, thresholdDB int) bool {

	_min := AbsInt16(slices.Min(pcm))
	_max := AbsInt16(slices.Max(pcm))

	return _min <= int16(thresholdDB) && _max <= int16(thresholdDB)

}

// AbsInt16 calculates the absolute value of an int16 using bitwise operations.
// This avoids a conditional jump, which can cause pipeline stalls, making it faster.
func AbsInt16(x int16) int16 {
	// 1. Create a mask:
	// If x >= 0, mask is 0.
	// If x < 0, mask is -1 (which is 0xFFFF in two's complement for int16).
	mask := x >> 15

	// 2. Perform the operation:
	// When x >= 0: (x ^ 0) - 0 = x
	// When x < 0: (x ^ (-1)) - (-1) = (^x) + 1.
	//    In two's complement, (~x) + 1 is the negative of x, or the absolute value.
	return (x ^ mask) - mask
}

func Abs(arr []int16) []int16 {
	result := make([]int16, len(arr))
	for i, val := range arr {
		result[i] = AbsInt16(val)
	}
	return result
}

// NoiseGateAAC computes the Average Absolute Change (AAC) of a signal to quantify signal volatility.
func NoiseGateAAC(arr []int16) float64 {
	var sumOfChanges int64 = 0

	for i := 1; i < len(arr); i++ {
		diff := arr[i] - arr[i-1]
		absDiff := AbsInt16(diff)
		sumOfChanges += int64(absDiff)
	}

	aac := float64(sumOfChanges) / float64(len(arr)-1)
	return aac
}

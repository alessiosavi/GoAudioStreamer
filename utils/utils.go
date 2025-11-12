package utils

import (
	"encoding/binary"
	"go-audio-streamer/constants"
	"math"
	"os"
	"runtime/pprof"
	"slices"

	"github.com/argusdusty/gofft"
	"github.com/gordonklaus/portaudio"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/constraints"
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

// NoiseGateFFT reduces background noise by zero-ing frequency bins below a threshold.
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

func Int16ToBytes(data []int16) []byte {
	ret := make([]byte, len(data))
	for i := range data {
		ret[i] = byte(data[i])
	}

	return ret
}

func BytesToInt16(data []byte) []int16 {
	ret := make([]int16, len(data))
	for i := range data {
		ret[i] = int16(data[i])
	}
	return ret
}

func SafeInt16SliceToByteSlice(data []int16) []byte {
	size := len(data) * 2
	buffer := make([]byte, size)

	for i, val := range data {
		// Explicitly write the 2 bytes of the int16 using Big Endian order
		binary.BigEndian.PutUint16(buffer[i*2:], uint16(val))
	}
	return buffer
}

func SafeByteSliceToInt16Slice(data []byte) []int16 {
	if len(data)%2 != 0 {
		// Handle odd length error
		panic("byte slice length is not even for int16 conversion")
	}

	size := len(data) / 2
	result := make([]int16, size)

	for i := 0; i < size; i++ {
		// Explicitly read 2 bytes using Big Endian order
		// and store the result in the int16 slice.
		result[i] = int16(binary.BigEndian.Uint16(data[i*2:]))
	}
	return result
}

func InitOutputStream(buffer *[]int16) *portaudio.Stream {
	outStream, err := portaudio.OpenDefaultStream(0, constants.Channels, float64(constants.SampleRate), constants.FrameSize, buffer)
	if err != nil {
		log.Errorf("Open output error: %v", err)
		return nil
	}
	if err = outStream.Start(); err != nil {
		outStream.Close()
		log.Errorf("Start output error: %v", err)
		return nil
	}
	log.Info("Output stream (re?)started")
	return outStream
}

func InitInputStream(buffer *[]int16) *portaudio.Stream {
	inStream, err := portaudio.OpenDefaultStream(constants.Channels, 0, float64(constants.SampleRate), constants.FrameSize, buffer)
	if err != nil {
		log.Errorf("Open input error: %v", err)
		return nil
	}

	if err = inStream.Start(); err != nil {
		inStream.Close()
		log.Errorf("Start input error: %v", err)
		return nil
	}
	log.Info("Input stream (re)started")
	return inStream
}

type Number interface {
	constraints.Integer | constraints.Float
}

// SlidingWindowAvg is a generic struct that calculates a moving average
// over a fixed window size. It uses an O(1) approach for updates.
type SlidingWindowAvg[T Number] struct {
	data    []T     // The array (slice) holding the current window elements
	maxSize int     // The fixed maximum size of the window
	sum     float64 // The running total sum of elements in 'data'
}

// NewSlidingWindowAvg creates and initializes a new SlidingWindowAvg struct.
func NewSlidingWindowAvg[T Number](size int) *SlidingWindowAvg[T] {
	return &SlidingWindowAvg[T]{
		data:    make([]T, 0, size),
		maxSize: size,
		sum:     0.0,
	}
}

// Push adds a new value to the window and efficiently updates the running sum.
// If the window is full, it removes the oldest element (pop) before adding the new one.
// This is the core O(1) optimization.
func (s *SlidingWindowAvg[T]) Push(item T) {
	// 1. Convert the generic item to float64 for sum calculation
	newItemValue := float64(item)

	// Check if the window is full (we reached maxSize)
	if len(s.data) == s.maxSize {
		// Window is full, we must remove the oldest element (at index 0)
		oldestValue := float64(s.data[0])

		// Remove the oldest element's value from the running sum (O(1) subtraction)
		s.sum -= oldestValue

		// Efficiently remove the oldest element from the slice (pop from front)
		// This is technically O(N) for slices, but remains the idiomatic way in Go.
		// For true O(1) pop/push, a circular buffer implementation would be used.
		s.data = s.data[1:]
	}

	// 2. Add the new item's value to the running sum (O(1) addition)
	s.sum += newItemValue

	// 3. Append the new item to the data slice
	s.data = append(s.data, item)

}

// Avg returns the current average of the elements in the window.
// It returns 0.0 if the window is empty.
func (s *SlidingWindowAvg[T]) Avg() float64 {
	if len(s.data) == 0 {
		return 0.0
	}
	// Calculate the average by dividing the running sum by the current count (O(1) calculation)
	return s.sum / float64(len(s.data))
}

func StartCPUProfiling(fname string) func() {
	f, err := os.Create(fname)
	if err != nil {
		log.Fatal("could not create CPU profile: ", err)
	}

	// 2. Start profiling
	if err := pprof.StartCPUProfile(f); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	// 3. Return a cleanup function
	return func() {
		pprof.StopCPUProfile()
		f.Sync()
		if err := f.Close(); err != nil {
			log.Printf("Profile Warning: error closing CPU profile file %s: %v", fname, err)
		}
		log.Printf("CPU profile successfully written to %s", fname)
	}
}

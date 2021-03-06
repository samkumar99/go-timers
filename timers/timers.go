package timers

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"
	)

/* HASHTABLE-BASED TIMERS */

var timers map[string]int64 = make(map[string]int64)
var timersEnd map[string]int64 = make(map[string]int64)

func StartTimer(name string) {
	if _, ok := timers[name]; ok {
		panic(fmt.Sprintf("Attempted to start running timer %s", name))
	} else {
		timers[name] = time.Now().UnixNano()
	}
}

func EndTimer(name string) {
	if _, ok := timersEnd[name]; ok {
		panic(fmt.Sprintf("Attempted to end stopped timer %s", name))
	} else {
		timersEnd[name] = time.Now().UnixNano()
	}
}

func GetTimerDelta(name string) int64 {
	if valStart, ok := timers[name]; ok {
		if valEnd, ok := timersEnd[name]; ok {
			return valEnd - valStart
		} else {
			return -2
		}
	} else {
		return -1
	}
}

func ResetTimer(name string) int64 {
	if val, ok := timers[name]; ok {
		now := time.Now().UnixNano()
		timers[name] = now
		return now - val
	} else {
		panic(fmt.Sprintf("Attempted to reset timer %s, which is not running", name))
	}
}

func PollTimer(name string) int64 {
	if val, ok := timers[name]; ok {
		return time.Now().UnixNano() - val
	} else {
		panic(fmt.Sprintf("Attempted to poll timer %s, which is not running", name))
	}
}

func DeleteTimer(name string) {
	if _, ok := timers[name]; ok {
		delete(timers, name)
	} else {
		panic(fmt.Sprintf("Attempted to stop timer %s, which is not running", name))
	}
	delete(timersEnd, name)
}

/* FILE-BASED TIMERS */

var timerDir string

func SetFileTimerCollection (dirString string) {
	fi, err := os.Stat(dirString)
	if err == nil && fi.IsDir() {
		lastIndex := len(dirString) - 1
		if dirString[lastIndex] == '/' {
			timerDir = dirString[0:lastIndex]
		} else {
			timerDir = dirString
		}
	} else {
		panic(fmt.Sprintf("Attempted to set Timer collection to invalid directory %s", dirString))
	}
}

func expandFilePathStart(name string) string {
	return fmt.Sprintf("%s/%s_start", timerDir, name)
}

func expandFilePathEnd(name string) string {
	return fmt.Sprintf("%s/%s_end", timerDir, name)
}

/** This will overwrite any existing timers. I didn't add error checking here
    because I reasoned that we may see some of the same timers from previous
    runs of the program. */
func StartFileTimer(name string) {
	writeFileTimer(name, expandFilePathStart)
}

func EndFileTimer(name string) {
	writeFileTimer(name, expandFilePathEnd)
}

func writeFileTimer(name string, nameFinder func (string) string) {
	file, err := os.Create(nameFinder(name))
	defer file.Close()
	if err == nil {
		err = binary.Write(file, binary.LittleEndian, time.Now().UnixNano())
		if err != nil {
			panic(fmt.Sprintf("Could not write to file timer %s: %v", nameFinder(name), err))
		}
	} else {
		panic(fmt.Sprintf("Could not write to file timer %s: %v", nameFinder(name), err))
	}
}

func readFileTimer(name string, nameFinder func (string) string) int64 {
	file, err := os.Open(nameFinder(name))
	defer file.Close()
	var fileTime int64
	if err == nil {
		err = binary.Read(file, binary.LittleEndian, &fileTime)
		if err != nil {
			panic(fmt.Sprintf("Could not poll file timer %s: %v", nameFinder(name), err))
		}
		return fileTime
	} else {
		panic(fmt.Sprintf("Could not open file timer %s: %v", nameFinder(name), err))
	}
}

func GetFileTimerDelta(name string) (delta int64) {
	var started bool = false
	defer func () {
			if r := recover(); r != nil {
				if started {
					delta = -2 // indicates timer was started but never ended
				} else {
				 	delta = -1 // indicates timer was never started
				}
			}
		}()
	var startTime int64 = readFileTimer(name, expandFilePathStart)
	started = true
	var endTime int64 = readFileTimer(name, expandFilePathEnd)
	delta = endTime - startTime
	return
}

func PollFileTimer(name string) int64 {
	return time.Now().UnixNano() - readFileTimer(name, expandFilePathStart)
}

func DeleteFileTimer(name string) {
	var err error = os.Remove(expandFilePathStart(name))
	if err != nil {
		panic(fmt.Sprintf("Could not stop file timer %s: %v", name, err))
	}
	os.Remove(expandFilePathEnd(name))
}

func DeleteFileTimerIfExists(name string) {
	os.Remove(expandFilePathStart(name))
	os.Remove(expandFilePathEnd(name))
}

/* LOG-BASED TIMERS */

var file *os.File = nil

func SetLogFile(filepath string) {
	if file != nil {
		file.Close()
	}
	var err error
	file, err = os.Create(filepath)
	if err != nil {
		panic(fmt.Sprintf("Attempted to set log to invalid filepath %v", err))
	}
}

func CloseLogFile() {
	if file == nil {
		panic(fmt.Sprintf("Attempted to close log file, but not log file is active"))
	} else {
		file.Sync()
		file.Close()
		file = nil
	}
}

func logEvent(name string, tag string) {
	_, err := file.WriteString(fmt.Sprintf("%s\x00%s", name, tag))
	if err == nil {
		err = binary.Write(file, binary.LittleEndian, time.Now().UnixNano())
		if err != nil {
			panic(fmt.Sprintf("Failed to write current time to file: %v", err))
		}
	} else {
		panic(fmt.Sprintf("Failed to write timer name to file: %v", err))
	}	
}

const (
	START_SYMBOL string = "s"
	END_SYMBOL string = "e"
	LEN_TYPE_SYMBOL int = 1 // both START_SYMBOL and END_SYMBOL have this length
	)

/** Name can't contain \0. */
func StartLogTimer(name string) {
	logEvent(name, START_SYMBOL)
}

func EndLogTimer(name string) {
	logEvent(name, END_SYMBOL)
}

type TimerSummary struct {
	starts []int64
	ends []int64
}

func checkerr(f *os.File, filename string, err error) {
	if err != nil {
		f.Close()
		if err == io.EOF {
			panic(fmt.Sprintf("Unexpected end of file when parsing %s", filename))
		} else {
			panic(fmt.Sprintf("Could not read file at filepath %s", filename))
		}
	}
}

func ParseFileToMap(filenames []string) map[string]*TimerSummary {
	var data [][]byte = make([][]byte, len(filenames))
	for i := 0; i < len(filenames); i++ {
		f, err := os.Open(filenames[i])
		if err != nil {
			f.Close()
			panic(fmt.Sprintf("Attempted to parse file at invalid filepath %s", filenames[i]))
		}
		data[i], err = ioutil.ReadAll(f) // it's OK to buffer everything in memory since I'm constructing a hashtable out of it anyway
		f.Close()
		if err != nil {
			panic(fmt.Sprintf("Could not read file at filepath %s", filenames[i]))
		}
	}
	var tmap map[string]*TimerSummary = make(map[string]*TimerSummary)
	var buf []byte = make([]byte, LEN_TYPE_SYMBOL, LEN_TYPE_SYMBOL)
	var name string
	var frag2 string
	var summary *TimerSummary
	var ok bool
	var time int64
	var freader *bufio.Reader
	var fname string
	
	for i := 0; i < len(filenames); i++ {
		fname = filenames[i]
		f, err := os.Open(fname)
		if err != nil {
			f.Close()
			panic(fmt.Sprintf("Attempted to parse file at invalid filepath %s", fname))
		}
		freader = bufio.NewReader(f)
		name, err = freader.ReadString('\x00')
		for err != io.EOF {
			name = name[:len(name) - 1]
			_, err = freader.Read(buf)
			checkerr(f, fname, err)
			frag2 = string(buf)
			err = binary.Read(freader, binary.LittleEndian, &time)
			checkerr(f, fname, err)
			summary, ok = tmap[name]
			if !ok {
				summary = &TimerSummary{make([]int64, 0, 1), make([]int64, 0, 1)}
				tmap[name] = summary
			}
			if frag2 == START_SYMBOL {
				summary.starts = append(summary.starts, time)
			} else {
				summary.ends = append(summary.ends, time)
			}
			name, err = freader.ReadString('\x00')
		}
		f.Close()
	}
	return tmap
}

func ParseMapToDeltas(tmap map[string]*TimerSummary) map[string][]int64 {
	var tname string
	var tsummary *TimerSummary
	var deltamap map[string][]int64 = make(map[string][]int64)
	var i int
	
	var deltas []int64
	
	TimerLoop:
		for tname, tsummary = range tmap {
			if len(tsummary.starts) == 0 {
				fmt.Printf("Timer %s was ended but never started\n", tname)
				continue
			} else if len(tsummary.ends) == 0 {
				fmt.Printf("Timer %s was started but never ended\n", tname)
				continue
			} else if len(tsummary.starts) != len(tsummary.ends) {
				fmt.Printf("Timer %s has a different number of starts than ends\n", tname)
				continue
			}
			deltas = make([]int64, len(tsummary.starts))
			for i = 0; i < len(tsummary.ends); i++ {
				if tsummary.starts[i] > tsummary.ends[i] {
					fmt.Printf("Timer %s has an end time preceding start time\n", tname)
					continue TimerLoop
				}
				if i > 0 && tsummary.starts[i] < tsummary.ends[i - 1] {
					fmt.Printf("Timer %s was started twice without being ended in between\n", tname)
					continue TimerLoop
				}
				deltas[i] = tsummary.ends[i] - tsummary.starts[i]
			}
			deltamap[tname] = deltas
		}
		
	return deltamap
}

/* BUFFERED LOG TIMER 
   An in-memory version of the log-based timer. Can be serialized to a log file. */

var bufferedTimers map[string]*TimerSummary = make(map[string]*TimerSummary)

func getSummary(name string) (summary *TimerSummary) {
	var exists bool
	summary, exists = bufferedTimers[name]
	if !exists {
		summary = &TimerSummary{make([]int64, 0, 7), make([]int64, 0, 7)}
		bufferedTimers[name] = summary
	}
	return
}

func StartBufferedLogTimer(name string) {
	var summary *TimerSummary = getSummary(name)
	summary.starts = append(summary.starts, time.Now().UnixNano())
}

func EndBufferedLogTimer(name string) {
	var summary *TimerSummary = getSummary(name)
	summary.ends = append(summary.ends, time.Now().UnixNano())
}

func writeArray(writer io.Writer, array []int64, name string, symbol string) error {
	var err error
	for i := 0; i < len(array); i++ {
		_, err = writer.Write([]byte(fmt.Sprintf("%s\x00%s", name, symbol)))
		if err != nil {
			return err
		}
		err = binary.Write(writer, binary.LittleEndian, array[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func WriteLogBuffer(writer io.Writer) error {
	var err error
	for name, summary := range bufferedTimers {
		err = writeArray(writer, summary.starts, name, START_SYMBOL)
		if err != nil {
			return err
		}
		err = writeArray(writer, summary.ends, name, END_SYMBOL)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetLogBuffer() map[string]*TimerSummary {
	return bufferedTimers
}

func ResetLogBuffer() {
	bufferedTimers = make(map[string]*TimerSummary)
}

func SetLogBuffer(newbuffer map[string]*TimerSummary) {
	bufferedTimers = newbuffer
}

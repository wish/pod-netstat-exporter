package netstat

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// NetStats is a mapping from stat name to value
type NetStats map[string]int64

// GetStats returns every netstat from /proc/net/(netstat|snmp|snmp6/sockstat/sockstat6)
func GetStats(rootFs string, pid int) (NetStats, error) {
	stats := NetStats{}
	nStats, err := netstatsFromProc(rootFs, pid, "net/netstat")
	if err != nil {
		return stats, err
	}
	for k, v := range nStats {
		stats[k] = v
	}

	sStats, err := netstatsFromProc(rootFs, pid, "net/snmp")
	if err != nil {
		return stats, err
	}
	for k, v := range sStats {
		stats[k] = v
	}

	s6Stats, err := snmp6FromProc(rootFs, pid, "net/snmp6")
	if err != nil {
		return stats, err
	}
	for k, v := range s6Stats {
		stats[k] = v
	}
	sockStats, err := sockstatsFromProc(rootFs, pid, "net/sockstat")
	if err != nil {
		return stats, err
	}
	for k, v := range sockStats {
		stats[k] = v
	}
	sock6Stats, err := sockstatsFromProc(rootFs, pid, "net/sockstat6")
	if err != nil {
		return stats, err
	}
	for k, v := range sock6Stats {
		stats[k] = v
	}
	return stats, nil
}

func netstatsFromProc(rootFs string, pid int, file string) (NetStats, error) {
	var err error
	var stats NetStats

	statsFile := path.Join(rootFs, "proc", strconv.Itoa(pid), file)

	r, err := os.Open(statsFile)
	if err != nil {
		return stats, fmt.Errorf("failure opening %s: %v", statsFile, err)
	}

	return parseNetStats(r)
}

// adapted from github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
func parseNetStats(r io.Reader) (NetStats, error) {
	var (
		netStats = NetStats{}
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		nameParts := strings.Split(scanner.Text(), " ")
		if !scanner.Scan() {
			logrus.Errorf("Odd number of lines in netstat file")
			break
		}
		valueParts := strings.Split(scanner.Text(), " ")
		// Remove trailing :.
		protocol := nameParts[0][:len(nameParts[0])-1]
		if len(nameParts) != len(valueParts) {
			return nil, fmt.Errorf("mismatch field count mismatch: %s",
				protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			v, err := strconv.Atoi(valueParts[i])
			if err != nil {
				continue
			}

			netStats[protocol+"_"+nameParts[i]] = int64(v)
		}
	}

	return netStats, scanner.Err()
}

func snmp6FromProc(rootFs string, pid int, file string) (NetStats, error) {
	var err error
	var stats NetStats

	statsFile := path.Join(rootFs, "proc", strconv.Itoa(pid), file)

	r, err := os.Open(statsFile)
	if err != nil {
		return stats, fmt.Errorf("failure opening %s: %v", statsFile, err)
	}

	return parseSNMP6Stats(r)
}

// adapted from github.com/prometheus/node_exporter/blob/master/collector/netstat_linux.go
func parseSNMP6Stats(r io.Reader) (NetStats, error) {
	var (
		netStats = NetStats{}
		scanner  = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		stat := strings.Fields(scanner.Text())
		if len(stat) < 2 {
			continue
		}
		// Expect to have "6" in metric name, skip line otherwise
		if sixIndex := strings.Index(stat[0], "6"); sixIndex != -1 {
			protocol := stat[0][:sixIndex+1]
			name := stat[0][sixIndex+1:]
			v, err := strconv.Atoi(stat[1])
			if err != nil {
				continue
			}
			netStats[protocol+"_"+name] = int64(v)
		}
	}

	return netStats, scanner.Err()
}

func sockstatsFromProc(rootFs string, pid int, file string) (NetStats, error) {
	var err error
	var stats NetStats

	statsFile := path.Join(rootFs, "proc", strconv.Itoa(pid), file)

	r, err := os.Open(statsFile)
	if err != nil {
		return stats, fmt.Errorf("failure opening %s: %v", statsFile, err)
	}

	return parseSockStats(r)
}
func parseSockStats(r io.Reader) (NetStats, error) {
	var stats = NetStats{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		fields := strings.Split(s.Text(), " ")
		metric := strings.TrimSuffix(fields[0], ":")
		if len(fields) < 3 {
			return nil, fmt.Errorf("Error: %s", s.Text())
		}
		for i := 1; i < len(fields); i++ {
			value, err := strconv.Atoi(fields[i+1])
			if err != nil {
				continue
			}
			stats[metric+"_"+fields[i]] = int64(value)
			i++
		}

	}
	return stats, nil
}

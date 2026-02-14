package evelogs

import (
	"regexp"
	"strings"
	"time"
)

var reportPattern = regexp.MustCompile(`^\[ ([0-9]{4}\.[0-9]{2}\.[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}) \] ([^>]+) > (.*)$`)

type ParsedReport struct {
	Time    time.Time
	Author  string
	Message string
}

func IsReportLine(line string) bool {
	return reportPattern.MatchString(line)
}

func ParseReportLine(line string) (ParsedReport, bool) {
	match := reportPattern.FindStringSubmatch(line)
	if len(match) < 4 {
		return ParsedReport{}, false
	}
	parsedTime, err := time.ParseInLocation("2006.01.02 15:04:05", match[1], time.UTC)
	if err != nil {
		return ParsedReport{}, false
	}
	return ParsedReport{
		Time:    parsedTime,
		Author:  strings.TrimSpace(match[2]),
		Message: strings.TrimSpace(match[3]),
	}, true
}

func ParseReportTime(line string) (time.Time, bool) {
	report, ok := ParseReportLine(line)
	if !ok {
		return time.Time{}, false
	}
	return report.Time, true
}

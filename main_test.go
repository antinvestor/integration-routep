package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test started when the test binary is started. Only calls main.
func TestSystem(t *testing.T) {

	// assert equality
	assert.Equal(t, 123, 123, "Just ensuring we are testing something")

}

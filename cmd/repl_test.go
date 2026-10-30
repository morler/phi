package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplCommandRejectsArguments(t *testing.T) {
	err := replCommand.Run([]string{"unexpected"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

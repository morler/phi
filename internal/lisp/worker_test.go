package lisp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeWorker(t *testing.T) {
	input := strings.NewReader(
		`"(def phi-worker-value 41)"` + "\n" + `"(+ phi-worker-value 1)"` + "\n" + `"(throw (Exception. \"boom\"))"` + "\n",
	)
	var output bytes.Buffer
	require.NoError(t, ServeWorker(input, &output))
	dec := json.NewDecoder(&output)
	var reply workerReply
	require.NoError(t, dec.Decode(&reply))
	assert.Empty(t, reply.Error)
	require.NoError(t, dec.Decode(&reply))
	assert.Equal(t, "42", reply.Result.Value)
	assert.Empty(t, reply.Error)
	require.NoError(t, dec.Decode(&reply))
	assert.Contains(t, reply.Error, "boom")
}

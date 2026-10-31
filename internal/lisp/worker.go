package lisp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type workerReply struct {
	Result Result
	Error  string
}

// ServeWorker runs a single Glojure instance over a private request stream.
// Its caller must isolate the process before exposing this protocol.
func ServeWorker(input io.Reader, output io.Writer) error {
	rt, err := New()
	if err != nil {
		return err
	}
	decoder, encoder := json.NewDecoder(input), json.NewEncoder(output)
	for {
		var code string
		if err := decoder.Decode(&code); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read evaluation request: %w", err)
		}
		result, evalErr := rt.Eval(context.Background(), code)
		reply := workerReply{Result: result}
		if evalErr != nil {
			reply.Error = evalErr.Error()
		}
		if err := encoder.Encode(reply); err != nil {
			return fmt.Errorf("write evaluation response: %w", err)
		}
	}
}

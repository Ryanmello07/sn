// Evidence bytes become owned only after the bounded read, response close and
// request lifecycle all succeed. Every caller retains its own status policy.
package main

import (
	"context"
	"errors"
	"io"
	"math"
)

// Close exactly once, including invalid bounds and canceled owners. Failed
// reads never return partial evidence; join read/close/cancellation causes.
func readEvidenceHttpBody(ctx context.Context, body io.ReadCloser, maximum int64) (raw []byte, err error) {
	if body == nil {
		return nil, errors.New("evidence response body is absent")
	}
	if ctx == nil {
		return nil, errors.Join(errors.New("evidence response context is absent"), body.Close())
	}
	defer func() {
		err = errors.Join(err, body.Close(), ctx.Err())
		if err != nil {
			raw = nil
		}
	}()
	if maximum < 0 || maximum == math.MaxInt64 {
		return nil, errors.New("evidence response byte limit is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err = io.ReadAll(io.LimitReader(body, maximum+1))
	if int64(len(raw)) > maximum {
		err = errors.Join(err, errors.New("response exceeds evidence size limit"))
	}
	return raw, err
}

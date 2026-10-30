package commitguard

import (
	"context"
	"errors"
	"testing"
)

func TestAdmissionCancellationAndDetachedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	guarded := WithAdmission(ctx, func(context.Context) error { return nil })
	admitted, err := Admit(guarded)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if admitted.Err() != nil {
		t.Fatalf("admitted context canceled: %v", admitted.Err())
	}
	if _, err := Admit(guarded); !errors.Is(err, context.Canceled) {
		t.Fatalf("admission after cancellation: %v", err)
	}
}

func TestAdmissionDenialAndOrdinaryContext(t *testing.T) {
	denied := WithAdmission(t.Context(), func(context.Context) error { return context.Canceled })
	if _, err := Admit(denied); !errors.Is(err, context.Canceled) {
		t.Fatalf("denied admission: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	admitted, err := Admit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if !errors.Is(admitted.Err(), context.Canceled) {
		t.Fatalf("ordinary context lost cancellation: %v", admitted.Err())
	}
}

func TestDenialWrapsAnyCauseAndMarksNotAttempted(t *testing.T) {
	cause := errors.New("control read failed")
	for _, setup := range []func() context.Context{
		func() context.Context {
			return WithAdmission(t.Context(), func(context.Context) error { return cause })
		},
		func() context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return WithAdmission(ctx, func(context.Context) error { t.Fatal("callback after cancellation"); return nil })
		},
	} {
		_, err := Admit(setup())
		var denied interface{ NotAttempted() bool }
		if !errors.As(err, &denied) || !denied.NotAttempted() || !(errors.Is(err, cause) || errors.Is(err, context.Canceled)) {
			t.Fatalf("denial: %v", err)
		}
	}
}

package chromedp

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
)

func TestCloseDialog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		accept     bool
		promptText string
		dialogType page.DialogType
		sel        string
		want       string
	}{
		{
			name:       "AlertAcceptWithPromptText",
			accept:     true,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypeAlert,
			sel:        "#alert",
			want:       "alert text",
		},
		{
			name:       "AlertDismissWithPromptText",
			accept:     false,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypeAlert,
			sel:        "#alert",
			want:       "alert text",
		},
		{
			name:       "AlertAcceptWithoutPromptText",
			accept:     true,
			dialogType: page.DialogTypeAlert,
			sel:        "#alert",
			want:       "alert text",
		},
		{
			name:       "AlertDismissWithoutPromptText",
			accept:     false,
			dialogType: page.DialogTypeAlert,
			sel:        "#alert",
			want:       "alert text",
		},
		{
			name:       "PromptAcceptWithPromptText",
			accept:     true,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypePrompt,
			sel:        "#prompt",
			want:       "prompt text",
		},
		{
			name:       "PromptDismissWithPromptText",
			accept:     false,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypePrompt,
			sel:        "#prompt",
			want:       "prompt text",
		},
		{
			name:       "PromptAcceptWithoutPromptText",
			accept:     true,
			dialogType: page.DialogTypePrompt,
			sel:        "#prompt",
			want:       "prompt text",
		},
		{
			name:       "PromptDismissWithoutPromptText",
			accept:     false,
			dialogType: page.DialogTypePrompt,
			sel:        "#prompt",
			want:       "prompt text",
		},
		{
			name:       "ConfirmAcceptWithPromptText",
			accept:     true,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypeConfirm,
			sel:        "#confirm",
			want:       "confirm text",
		},
		{
			name:       "ConfirmDismissWithPromptText",
			accept:     false,
			promptText: "this is a prompt text",
			dialogType: page.DialogTypeConfirm,
			sel:        "#confirm",
			want:       "confirm text",
		},
		{
			name:       "ConfirmAcceptWithoutPromptText",
			accept:     true,
			dialogType: page.DialogTypeConfirm,
			sel:        "#confirm",
			want:       "confirm text",
		},
		{
			name:       "ConfirmDismissWithoutPromptText",
			accept:     false,
			dialogType: page.DialogTypeConfirm,
			sel:        "#confirm",
			want:       "confirm text",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := testAllocate(t, "")
			defer cancel()

			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()
			opening := Events(tctx, page.JavascriptDialogOpening)
			closed := Events(tctx, page.JavascriptDialogClosed)

			go func() {
				for e, err := range opening {
					if err != nil {
						return
					}
					if e.Type != test.dialogType {
						t.Errorf("expected dialog type to be %q, got: %q", test.dialogType, e.Type)
					}
					if e.Message != test.want {
						t.Errorf("expected dialog message to be %q, got: %q", test.want, e.Message)
					}

					// Handle the dialog from another goroutine than the one
					// that receives the events.
					go func() {
						if err := Do(ctx, Func(func(ctx context.Context, t *Target) error {
							_, err := cdp.Call(ctx, t, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{
								Accept:     test.accept,
								PromptText: test.promptText,
							})
							return err
						})); err != nil && tctx.Err() == nil {
							t.Error(err)
						}
					}()
				}
			}()

			if err := Do(ctx,
				Navigate(testdataDir+"/dialog.html"),
				Click(ID(test.sel), NodeVisible),
			); err != nil {
				t.Fatal(err)
			}

			for e, err := range closed {
				if err != nil {
					t.Fatal(err)
				}
				if e.Result != test.accept {
					t.Errorf("expected result to be %t, got %t", test.accept, e.Result)
				}
				if e.UserInput != test.promptText {
					t.Errorf("expected user input to be %q, got %q", test.promptText, e.UserInput)
				}
				break
			}
		})
	}
}

func TestWaitNewTarget(t *testing.T) {
	t.Parallel()

	ctx, cancel := testAllocate(t, "newtab.html")
	defer cancel()

	ch := WaitNewTarget(ctx, func(info *target.Info) bool {
		return info.URL != ""
	})
	if err := Do(ctx, Click(ID("new-tab"))); err != nil {
		t.Fatal(err)
	}
	blankCtx, cancel := NewContext(ctx, WithTargetID(<-ch))
	defer cancel()

	var urlstr string
	if err := Do(blankCtx,
		into(&urlstr, Location()),
		WaitVisible(ID(`form`)),
	); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(urlstr, "form.html") {
		t.Errorf("want to be on form.html, at %q", urlstr)
	}
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	ctx, cancel := testAllocate(t, "")
	defer cancel()
	if err := Do(ctx); err != nil {
		t.Fatal(err)
	}

	tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
	defer tcancel()

	// The subscription starts when Events returns, so the event of the
	// navigation below is not lost before the loop starts.
	events := cdp.Events(tctx, FromContext(ctx).Target, page.LoadEventFired)
	if err := Do(ctx, Func(func(ctx context.Context, t *Target) error {
		_, err := cdp.Call(ctx, t, page.Navigate, page.NavigateParams{URL: testdataDir + "/form.html"})
		return err
	})); err != nil {
		t.Fatal(err)
	}

	for ev, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		if ev.Timestamp == 0 {
			t.Error("the load event has no timestamp")
		}
		return
	}
	t.Error("the subscription ended without a load event")
}

func TestListen(t *testing.T) {
	t.Parallel()

	ctx, cancel := testAllocate(t, "")
	defer cancel()
	if err := Do(ctx); err != nil {
		t.Fatal(err)
	}

	// The browser sends Runtime.consoleAPICalled while it runs the
	// expression, so before the reply to Runtime.evaluate. The listener runs
	// in the event loop, so it has the event when Run returns, with no wait.
	var mu sync.Mutex
	var got []string
	replies := 0
	lctx, lcancel := context.WithCancel(ctx)
	FromContext(ctx).Target.Listen(lctx, func(ev any) {
		mu.Lock()
		defer mu.Unlock()
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			if len(ev.Args) == 1 {
				got = append(got, string(ev.Args[0].Value))
			}
		case *cdproto.Message:
			replies++
		}
	})
	for _, s := range []string{"one", "two"} {
		if err := Do(ctx, Evaluate[Void](`console.log("`+s+`")`)); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if want := `"` + s + `"`; n == 0 || got[n-1] != want {
			t.Fatalf("when Run returned, the listener had %q, want %s last", got, want)
		}
	}
	mu.Lock()
	if replies != 0 {
		t.Errorf("the listener got %d replies to commands, want only events", replies)
	}
	mu.Unlock()

	// After the cancel, the listener gets nothing more.
	lcancel()
	if err := Do(ctx, Evaluate[Void](`console.log("three")`)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Errorf("after the cancel, the listener had %q, want the two events before it", got)
	}
}

func TestCallInvalidContext(t *testing.T) {
	t.Parallel()

	if _, err := Call(context.Background(), page.GetFrameTree, cdp.Empty{}); err != ErrInvalidContext {
		t.Errorf("Call: got error %v, want %v", err, ErrInvalidContext)
	}
	if _, err := CallBrowser(context.Background(), target.GetTargets, target.GetTargetsParams{}); err != ErrInvalidContext {
		t.Errorf("CallBrowser: got error %v, want %v", err, ErrInvalidContext)
	}
}

func TestCallBrowserError(t *testing.T) {
	t.Parallel()

	ctx, cancel := testAllocate(t, "")
	defer cancel()

	_, err := CallBrowser(ctx, target.CloseTarget, target.CloseTargetParams{TargetID: "no-such-target"})
	var cdpErr *cdproto.Error
	if !errors.As(err, &cdpErr) {
		t.Errorf("got error %v (%T), want a *cdproto.Error", err, err)
	}
}

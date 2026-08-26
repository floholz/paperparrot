// Dev helper: drive the running UI with headless Chrome, log console errors, save screenshots.
// go run ./tools/uishot http://127.0.0.1:8099 t@example.com password123 <outdir> [#/route ...]
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func main() {
	base, email, pass, out := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	routes := os.Args[5:]
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", "new"), chromedp.WindowSize(1440, 900))
	if p := os.Getenv("PP_CHROME"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	actx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel2 := chromedp.NewContext(actx)
	defer cancel2()
	ctx, cancel3 := context.WithTimeout(ctx, 300*time.Second)
	defer cancel3()

	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventExceptionThrown:
			desc := ""
			if e.ExceptionDetails.Exception != nil {
				desc = e.ExceptionDetails.Exception.Description
			}
			fmt.Println("EXCEPTION:", e.ExceptionDetails.Text, desc)
		case *runtime.EventConsoleAPICalled:
			if e.Type == "error" || e.Type == "warning" {
				var parts []string
				for _, a := range e.Args {
					parts = append(parts, string(a.Value)+" "+a.Description)
				}
				fmt.Println("CONSOLE", e.Type+":", strings.Join(parts, " "))
			}
		}
	})

	shot := func(name string) chromedp.Action {
		return chromedp.ActionFunc(func(ctx context.Context) error {
			var buf []byte
			if err := chromedp.CaptureScreenshot(&buf).Do(ctx); err != nil {
				return err
			}
			p := filepath.Join(out, name+".png")
			fmt.Println("shot", p)
			return os.WriteFile(p, buf, 0o644)
		})
	}
	must := func(err error) {
		if err != nil {
			fmt.Println("ERROR:", err)
			os.Exit(1)
		}
	}
	must(chromedp.Run(ctx,
		chromedp.Navigate(base+"/#/templates"),
		chromedp.Sleep(1500*time.Millisecond),
		shot("00-login"),
		chromedp.WaitVisible(`input[type=email]`),
		chromedp.SendKeys(`input[type=email]`, email),
		chromedp.SendKeys(`input[type=password]`, pass),
		chromedp.Click(`button.primary`),
		chromedp.WaitVisible(`header.top`),
		chromedp.Sleep(800*time.Millisecond),
		shot("01-templates"),
	))
	for i, r := range routes {
		name := fmt.Sprintf("%02d-%s", i+2, strings.NewReplacer("#", "", "/", "_", "?", "_", "=", "_").Replace(r))
		must(chromedp.Run(ctx,
			chromedp.Navigate(base+"/"+r),
			chromedp.Sleep(2500*time.Millisecond),
			shot(name),
		))
		if strings.Contains(r, "/documents/") || strings.Contains(r, "/templates/") {
			// PDF toggle
			must(chromedp.Run(ctx,
				chromedp.Click(`.preview .seg button:nth-child(2)`),
				chromedp.Sleep(3000*time.Millisecond),
				shot(name+"-pdf"),
			))
		}
	}
	fmt.Println("done")
}

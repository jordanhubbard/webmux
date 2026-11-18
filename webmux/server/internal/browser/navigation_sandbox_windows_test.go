//go:build windows

package browser

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/systeminfo"
	"github.com/chromedp/chromedp"
	"golang.org/x/sys/windows"
)

// Called only after the fixture trial and trace flush. IDs stay private; the
// artifact contains only process cardinality and the primary-token result.
func inspectFirstNavigationNetworkSandbox(root context.Context) *firstNavigationNetworkSandbox {
	report := &firstNavigationNetworkSandbox{}
	ctx, cancel := context.WithTimeout(root, 2*time.Second)
	defer cancel()
	info, err := cdp.Call(ctx, chromedp.FromContext(root).Browser, systeminfo.GetProcessInfo, cdp.Empty{})
	if err != nil {
		report.ErrorClass = "process_info_error"
		return report
	}
	var pid uint32
	for _, p := range info.ProcessInfo {
		if p.Type != "network.mojom.NetworkService" {
			continue
		}
		report.ProcessCount++
		if p.ID <= 0 || p.ID > 0xffffffff {
			report.ErrorClass = "invalid_process_id"
			return report
		}
		pid = uint32(p.ID)
	}
	if report.ProcessCount != 1 {
		report.ErrorClass = "network_process_missing_or_ambiguous"
		return report
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		report.ErrorClass = "process_query_error"
		return report
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		report.ErrorClass = "token_query_error"
		return report
	}
	defer token.Close()
	// TOKEN_INFORMATION_CLASS::TokenIsAppContainer (29); OpenProcessToken
	// returns a primary token, not the identity-level impersonation exception.
	const tokenIsAppContainer = 29
	var value [4]byte
	var size uint32
	if err := windows.GetTokenInformation(token, tokenIsAppContainer, &value[0], 4, &size); err != nil || size != 4 {
		report.ErrorClass = "token_information_error"
		return report
	}
	report.Observed = true
	report.AppContainer = binary.LittleEndian.Uint32(value[:]) != 0
	return report
}

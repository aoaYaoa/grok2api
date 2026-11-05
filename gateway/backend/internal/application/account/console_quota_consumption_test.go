package account

import (
	"context"
	"testing"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
)

func TestRequestCountDecrementPreservesConsoleMonetaryQuota(t *testing.T) {
	for _, mode := range []string{"console", "console_image", "console_video", accountdomain.QuotaModeWebVideo} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC()
			service, repo := newAutoCleanTestService(t, now)
			console := mode != accountdomain.QuotaModeWebVideo
			provider := accountdomain.ProviderWeb
			if console {
				provider = accountdomain.ProviderConsole
			}
			credential := mustUpsert(t, repo, accountdomain.Credential{
				Provider: provider, AuthType: accountdomain.AuthTypeSSO, Name: mode, SourceKey: mode,
				EncryptedAccessToken: "test", Enabled: true, AuthStatus: accountdomain.AuthStatusActive,
			})
			if err := repo.SaveQuotaWindows(ctx, credential.ID, accountdomain.WebTierBasic, now, []accountdomain.QuotaWindow{{
				AccountID: credential.ID, Mode: mode, Remaining: 1, Total: 100, SyncedAt: &now, Source: accountdomain.QuotaSourceUpstream,
			}}); err != nil {
				t.Fatal(err)
			}
			updated, err := service.DecrementQuota(ctx, credential.ID, mode, 1)
			if err != nil || updated == console {
				t.Fatalf("decrement updated=%v error=%v; Console money must not be charged as request counts", updated, err)
			}
			if console {
				if updated, err := service.DecrementWebQuota(ctx, credential.ID, mode, 3); err != nil || updated {
					t.Fatalf("image finalization changed monetary quota: updated=%v error=%v", updated, err)
				}
			}
			windows, err := repo.GetQuotaWindows(ctx, []uint64{credential.ID})
			if err != nil {
				t.Fatal(err)
			}
			wantRemaining := 0
			if console {
				wantRemaining = 1
			}
			if len(windows[credential.ID]) != 1 || windows[credential.ID][0].Remaining != wantRemaining {
				t.Fatalf("quota windows=%#v, want remaining=%d", windows[credential.ID], wantRemaining)
			}
		})
	}
}

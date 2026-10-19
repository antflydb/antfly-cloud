package main

import "testing"

func TestLiteCreationCarriesPriceConsentWithoutChangingStandard(t *testing.T) {
	for _, intro := range []bool{false, true} {
		req := liteCreateRequest("test", "test", "us-central1", "50Gi", true, intro)
		if req.Tier != "lite" || req.Mode != "single" || req.DeploymentProfile != "lite" || req.VersionPolicy != "pinned" || req.NodeConfig.DataStorage != "50Gi" || !req.AcceptLiteTerms || req.ClaimLiteIntro != intro {
			t.Fatalf("unexpected Lite request: %+v", req)
		}
	}
}

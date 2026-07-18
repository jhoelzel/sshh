package main

import "testing"

func TestProcessTreeRSSIncludesOnlyRootAndDescendants(t *testing.T) {
	data := []byte(`
  1   0 100
 10   1 200
 11  10 300
 12  11 400
 20   1 500
 21  20 600
`)
	got, count, webKitCount, err := processTreeRSS(10, data, nil)
	if err != nil {
		t.Fatalf("calculate process tree RSS: %v", err)
	}
	if want := uint64((200 + 300 + 400) * 1024); got != want {
		t.Fatalf("process tree RSS = %d, want %d", got, want)
	}
	if count != 3 {
		t.Fatalf("process tree count = %d, want 3", count)
	}
	if webKitCount != 0 {
		t.Fatalf("WebKit process count = %d, want 0", webKitCount)
	}
}

func TestProcessTreeRSSRejectsMissingRoot(t *testing.T) {
	if _, _, _, err := processTreeRSS(99, []byte("1 0 100\n"), nil); err == nil {
		t.Fatal("missing benchmark root process was accepted")
	}
}

func TestProcessTreeRSSIncludesOnlyNewWebKitHelpers(t *testing.T) {
	data := []byte(`
 10 1 200 /tmp/shhh
 11 10 300 /tmp/shhh --fixture
 20 1 400 /System/Library/Frameworks/WebKit.framework/XPCServices/com.apple.WebKit.WebContent
 21 1 500 /System/Library/Frameworks/WebKit.framework/XPCServices/com.apple.WebKit.GPU
`)
	rss, count, webKitCount, err := processTreeRSS(10, data, map[int]bool{20: true})
	if err != nil {
		t.Fatalf("calculate WebKit process RSS: %v", err)
	}
	if want := uint64((200 + 300 + 500) * 1024); rss != want || count != 3 || webKitCount != 1 {
		t.Fatalf("usage = %d bytes, %d processes, %d WebKit; want %d, 3, 1", rss, count, webKitCount, want)
	}
}

package analysis

import "testing"

func TestRegisterStoresRuleAndDoc(t *testing.T) {
	resetRegistryForTest(t)
	r := &fakeRule{}
	doc := "# fake_v1\n## What it detects\nA fake.\n"
	Register(r, doc)

	codes := RegisteredCodes()
	if len(codes) != 1 || codes[0] != "diagnostic.fake_v1" {
		t.Fatalf("RegisteredCodes() = %v, want [diagnostic.fake_v1]", codes)
	}
	if got := GetDoc("diagnostic.fake_v1"); got != doc {
		t.Errorf("GetDoc(diagnostic.fake_v1) = %q", got)
	}
	if got := Lookup("diagnostic.fake_v1"); got != r {
		t.Errorf("Lookup(diagnostic.fake_v1) = %v, want %v", got, r)
	}
	if got := GetMeta("diagnostic.fake_v1"); got.Code != "fake" {
		t.Errorf("GetMeta(diagnostic.fake_v1) = %+v", got)
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	resetRegistryForTest(t)
	Register(&fakeRule{}, "doc")
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on duplicate Register, got nil")
		}
	}()
	Register(&fakeRule{}, "doc")
}

func TestRegisterRejectsEmptyDoc(t *testing.T) {
	resetRegistryForTest(t)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on empty doc, got nil")
		}
	}()
	Register(&fakeRule{}, "")
}

package auth

import "testing"

func TestNeedCaptchaAfterTwoFailsInWindow(t *testing.T) {
	s := New(nil)
	if s.NeedCaptcha("1.2.3.4") {
		t.Fatal("fresh IP")
	}
	s.Fail("1.2.3.4")
	if s.NeedCaptcha("1.2.3.4") {
		t.Fatal("one failure should not require captcha")
	}
	s.Fail("1.2.3.4")
	if !s.NeedCaptcha("1.2.3.4") {
		t.Fatal("two failures in 30m should require captcha")
	}
	s.OK("1.2.3.4")
	if s.NeedCaptcha("1.2.3.4") {
		t.Fatal("success should clear captcha")
	}
}

func TestCaptchaRoundTrip(t *testing.T) {
	s := New(nil)
	ch, err := s.IssueCaptcha("10.0.0.1")
	if err != nil || ch.ID == "" || ch.Image == "" {
		t.Fatalf("issue: %#v %v", ch, err)
	}
	s.mu.Lock()
	ans := s.captcha[ch.ID].answer
	s.mu.Unlock()
	if !s.CheckCaptcha("10.0.0.1", ch.ID, ans) {
		t.Fatal("valid answer rejected")
	}
	if s.CheckCaptcha("10.0.0.1", ch.ID, ans) {
		t.Fatal("captcha should be one-time")
	}
}

func TestCaptchaBoundToIP(t *testing.T) {
	s := New(nil)
	ch, err := s.IssueCaptcha("10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	ans := s.captcha[ch.ID].answer
	s.mu.Unlock()
	if s.CheckCaptcha("10.0.0.2", ch.ID, ans) {
		t.Fatal("other IP should not use captcha")
	}
}

package hosting

import (
	"strings"
	"testing"
)

const wpConfigSample = `<?php
define( 'DB_NAME', 'app' );
/* That's all, stop editing! Happy publishing. */
require_once ABSPATH . 'wp-settings.php';
`

func TestWPCronState(t *testing.T) {
	on, system := wpCronState(false, wpConfigSample)
	if !on || system {
		t.Fatalf("visit cron should stay on: on=%v system=%v", on, system)
	}
	on, system = wpCronState(false, "define( 'DISABLE_WP_CRON', true );\n")
	if on || system {
		t.Fatal("disabled constant should turn cron off")
	}
	on, system = wpCronState(true, "define('DISABLE_WP_CRON', true);")
	if !on || !system {
		t.Fatal("system job should count as on")
	}
	if !wpCronDisabled("define( \"DISABLE_WP_CRON\", 1 );") {
		t.Fatal("numeric true should disable cron")
	}
	if wpCronDisabled("define( 'DISABLE_WP_CRON', false );") {
		t.Fatal("false should leave cron on")
	}
}

func TestSetWPCronDisabled(t *testing.T) {
	got := setWPCronDisabled(wpConfigSample, true)
	if !wpCronDisabled(got) {
		t.Fatal("expected the constant to be inserted")
	}
	if i, j := strings.Index(got, "DISABLE_WP_CRON"), strings.Index(got, "That's all"); i < 0 || j < i {
		t.Fatalf("constant should sit before the stop marker:\n%s", got)
	}
	again := setWPCronDisabled(got, true)
	if again != got {
		t.Fatal("writing the same value should not duplicate the constant")
	}
	off := setWPCronDisabled("define( 'DISABLE_WP_CRON', false );\n", true)
	if !wpCronDisabled(off) || strings.Count(off, "DISABLE_WP_CRON") != 1 {
		t.Fatalf("replace existing constant: %s", off)
	}
}

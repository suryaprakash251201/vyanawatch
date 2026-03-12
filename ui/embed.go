package ui

import _ "embed"

//go:embed templates/dashboard.html
var DashboardHTML []byte

//go:embed templates/status.html
var StatusHTML []byte

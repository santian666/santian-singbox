package main

import "strings"

func incompatibleWinsockProvider(output string) string {
	text := strings.ToLower(output)
	for _, marker := range []string{
		"asproxy64.dll",
		"asproxy.dll",
		"asproxy layered service provider",
		"asproxy over tcp",
		"asproxy over udp",
	} {
		if strings.Contains(text, marker) {
			return "Astrill ASProxy Winsock LSP"
		}
	}
	return ""
}

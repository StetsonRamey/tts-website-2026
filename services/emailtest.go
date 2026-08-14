package services

// Test-recipient override for automated customer emails.
//
// When EMAIL_TEST_TO is set, every customer-facing email sent by the
// estimate, confirmation, and oos handlers is delivered to that address
// instead of the lead's email from Airtable. Empty/unset = normal behavior.
// Used to preview real production emails before they go to live clients.

import (
	"log"
	"os"
)

// resolveRecipient returns the address the email should actually be sent to.
// If EMAIL_TEST_TO is set, it overrides the lead's address and logs the
// redirect loudly so test mode is obvious in the journal.
func resolveRecipient(leadEmail string) string {
	if override := os.Getenv("EMAIL_TEST_TO"); override != "" {
		log.Printf("[email] TEST MODE (EMAIL_TEST_TO): redirecting %s -> %s", leadEmail, override)
		return override
	}
	return leadEmail
}

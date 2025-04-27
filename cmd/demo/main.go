package main

import (
	"errors"
	"fmt"
	"log"

	espoclient "github.com/egorsmkv/go-espo-api-client"
)

func main() {
	yourEspoURL := "https://your-espo-instance.com" // CHANGE THIS
	apiKey := "your-api-key"                        // CHANGE THIS
	// secretKey := "your-secret-key"                  // CHANGE THIS (if using HMAC)

	// Create a new client
	client, err := espoclient.NewClient(yourEspoURL, nil) // Use nil for default port
	if err != nil {
		log.Fatalf("Error creating client: %v", err)
	}

	// Set authentication - Choose one method:
	// client.SetUsernameAndPassword("admin", "password") // Basic Auth (not recommended)
	client.SetApiKey(apiKey)
	// client.SetSecretKey(secretKey) // Uncomment if using HMAC

	// --- Example: Create a Lead (POST request) ---
	fmt.Println("Attempting to create a Lead...")
	leadData := map[string]any{
		"firstName":    "John",
		"lastName":     "Doe",
		"emailAddress": "john.doe.test@example.com",
		"status":       "New", // Make sure required fields are included
		// Add other fields as needed based on your EspoCRM entity definition
	}

	// Optional headers
	headers := map[string]string{
		"X-Skip-Duplicate-Check": "true",
	}

	resp, err := client.Request(espoclient.MethodPost, "Lead", leadData, headers)

	if err != nil {
		// Check if it's a specific API response error
		var respErr *espoclient.ResponseError
		if errors.As(err, &respErr) {
			// It's an error response from the API (non-2xx status)
			fmt.Printf("API Error Occurred:\n")
			fmt.Printf("  Status Code: %d\n", respErr.Response.StatusCode)
			fmt.Printf("  Reason Header: %s\n", respErr.ErrorMessage) // X-Status-Reason
			fmt.Printf("  Response Body: %s\n", respErr.Response.GetBodyString())

			// You can still access the full response details via respErr.Response
			// fmt.Printf("  Content-Type: %s\n", respErr.Response.ContentType)
			// fmt.Printf("  Headers: %v\n", respErr.Response.Headers)
		} else {
			// It's another kind of error (network, setup, marshalling, etc.)
			log.Fatalf("Client or network error: %v", err)
		}
		return // Stop execution after error
	}

	// --- Success ---
	fmt.Printf("Lead Creation Successful!\n")
	fmt.Printf("  Status Code: %d\n", resp.StatusCode)
	fmt.Printf("  Content-Type: %s\n", resp.ContentType)
	// fmt.Printf("  Response Headers: %v\n", resp.Headers) // Uncomment to see headers

	// Attempt to parse the response body as JSON
	var createdLead map[string]any // Or define a specific struct
	parseErr := resp.GetParsedBody(&createdLead)
	if parseErr != nil {
		fmt.Printf("  Could not parse JSON response body: %v\n", parseErr)
		fmt.Printf("  Raw Response Body: %s\n", resp.GetBodyString())
	} else {
		fmt.Printf("  Parsed Response Body: %+v\n", createdLead)
		// Access fields like createdLead["id"]
		if id, ok := createdLead["id"]; ok {
			fmt.Printf("  Created Lead ID: %v\n", id)
		}
	}

	// --- Example: Get the created Lead (GET request) ---
	if createdLead != nil {
		if id, ok := createdLead["id"].(string); ok && id != "" {
			fmt.Printf("\nAttempting to retrieve Lead %s...\n", id)
			getResp, getErr := client.Request(espoclient.MethodGet, "Lead/"+id, nil, nil) // No data or headers for basic GET

			if getErr != nil {
				// Handle error similarly to the POST request
				var respErr *espoclient.ResponseError
				if errors.As(getErr, &respErr) {
					fmt.Printf("API Error Occurred on GET:\n")
					fmt.Printf("  Status Code: %d\n", respErr.Response.StatusCode)
					fmt.Printf("  Response Body: %s\n", respErr.Response.GetBodyString())
				} else {
					log.Fatalf("Client or network error on GET: %v", getErr)
				}
			} else {
				fmt.Printf("Lead Retrieval Successful!\n")
				fmt.Printf("  Status Code: %d\n", getResp.StatusCode)
				var retrievedLead map[string]any
				parseErr := getResp.GetParsedBody(&retrievedLead)
				if parseErr != nil {
					fmt.Printf("  Could not parse GET response body: %v\n", parseErr)
					fmt.Printf("  Raw Response Body: %s\n", getResp.GetBodyString())
				} else {
					fmt.Printf("  Retrieved Lead Data: %+v\n", retrievedLead)
				}
			}
		}
	}
}

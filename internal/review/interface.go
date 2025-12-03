package review

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"pudl/internal/errors"
	"pudl/internal/schema"
)

// InteractiveReviewer handles the interactive review workflow
type InteractiveReviewer struct {
	session           *ReviewSession
	sessionMgr        *SessionManager
	schemaMgr         *schema.Manager
	validator         *schema.Validator
	validationService *ValidationService
	schemaCreator     *SchemaCreator
	catalogUpdater    *CatalogUpdater
	reader            *bufio.Reader
}

// NewInteractiveReviewer creates a new interactive reviewer
func NewInteractiveReviewer(session *ReviewSession, sessionMgr *SessionManager, schemaMgr *schema.Manager, validator *schema.Validator, catalogUpdater *CatalogUpdater, schemaPath string) (*InteractiveReviewer, error) {
	// Create validation service
	validationService, err := NewValidationService(schemaPath)
	if err != nil {
		return nil, err
	}

	return &InteractiveReviewer{
		session:           session,
		sessionMgr:        sessionMgr,
		schemaMgr:         schemaMgr,
		validator:         validator,
		validationService: validationService,
		schemaCreator:     NewSchemaCreator(schemaMgr, validator, schemaPath),
		catalogUpdater:    catalogUpdater,
		reader:            bufio.NewReader(os.Stdin),
	}, nil
}

// RunReview starts the interactive review workflow
func (ir *InteractiveReviewer) RunReview() error {
	fmt.Printf("🔍 Starting schema review session: %s\n", ir.session.SessionID)
	ir.showProgressHeader()

	for !ir.session.IsComplete() {
		item := ir.session.GetCurrentItem()
		if item == nil {
			break
		}

		if err := ir.reviewItem(item); err != nil {
			return err
		}

		// Save session state after each item
		if err := ir.sessionMgr.SaveSession(ir.session); err != nil {
			fmt.Printf("⚠️  Warning: Failed to save session state: %v\n", err)
		}

		// Show progress update
		ir.showProgressUpdate()
	}

	// Mark session as complete
	ir.session.State = SessionCompleted
	if err := ir.sessionMgr.SaveSession(ir.session); err != nil {
		return err
	}

	// Apply changes to catalog
	if len(ir.session.Changes) > 0 {
		fmt.Printf("💾 Applying changes to catalog...\n")
		updateSummary, err := ir.catalogUpdater.ApplySessionChanges(ir.session)
		if err != nil {
			fmt.Printf("⚠️  Warning: Failed to apply some changes: %v\n", err)
		} else {
			fmt.Printf("✅ Applied %d changes to catalog\n", updateSummary.SuccessfulUpdates)
			if updateSummary.FailedUpdates > 0 {
				fmt.Printf("⚠️  %d changes failed to apply\n", updateSummary.FailedUpdates)
				for _, errMsg := range updateSummary.Errors {
					fmt.Printf("   • %s\n", errMsg)
				}
			}
		}
		fmt.Println()
	}

	ir.showSummary()
	return nil
}

// reviewItem handles the review of a single item
func (ir *InteractiveReviewer) reviewItem(item *ReviewItem) error {
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("📄 Item %d/%d: %s\n", ir.session.CurrentIndex+1, len(ir.session.Items), item.EntryID)
	fmt.Printf("📋 Current Schema: %s\n", item.CurrentSchema)
	if item.SuggestedSchema != "" && item.SuggestedSchema != item.CurrentSchema {
		fmt.Printf("💡 Suggested Schema: %s\n", item.SuggestedSchema)
	}
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")

	// Show data preview
	ir.showDataPreview(item.Data)

	for {
		choice, err := ir.promptChoice()
		if err != nil {
			return err
		}

		switch choice {
		case "accept", "a":
			return ir.acceptSchema(item)
		case "reassign", "r":
			if err := ir.reassignSchema(item); err != nil {
				fmt.Printf("❌ Error: %v\n\n", err)
				continue
			}
			return nil
		case "create", "c":
			if err := ir.createSchema(item); err != nil {
				fmt.Printf("❌ Error: %v\n\n", err)
				continue
			}
			return nil
		case "skip", "s":
			return ir.skipItem(item)
		case "view", "v":
			ir.viewFullData(item)
		case "back", "b":
			return ir.goBack()
		case "quit", "q":
			return ir.quitReview()
		case "help", "h", "?":
			ir.showHelp()
		default:
			fmt.Printf("❌ Invalid choice. Type 'help' for available options.\n\n")
		}
	}
}

// showDataPreview displays a preview of the data
func (ir *InteractiveReviewer) showDataPreview(data interface{}) {
	fmt.Printf("\n📋 Data Preview:\n")
	fmt.Printf("───────────────────────────────────────────────────────────────\n")

	// Convert to JSON for pretty printing
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error displaying data: %v\n", err)
		return
	}

	// Limit preview to first 20 lines
	lines := strings.Split(string(jsonData), "\n")
	maxLines := 20
	if len(lines) > maxLines {
		for i := 0; i < maxLines; i++ {
			fmt.Printf("%s\n", lines[i])
		}
		fmt.Printf("... (%d more lines)\n", len(lines)-maxLines)
	} else {
		fmt.Printf("%s\n", string(jsonData))
	}
	fmt.Printf("───────────────────────────────────────────────────────────────\n\n")
}

// promptChoice prompts the user for their choice
func (ir *InteractiveReviewer) promptChoice() (string, error) {
	fmt.Printf("Choose an action:\n")
	fmt.Printf("  [a]ccept    - Keep current schema\n")
	fmt.Printf("  [r]eassign  - Assign to different schema\n")
	fmt.Printf("  [c]reate    - Create new schema from this data\n")
	fmt.Printf("  [s]kip      - Skip this item\n")
	fmt.Printf("  [v]iew      - View full data in pager\n")
	fmt.Printf("  [b]ack      - Go back to previous item\n")
	fmt.Printf("  [q]uit      - Quit review session\n")
	fmt.Printf("  [h]elp      - Show this help\n")
	fmt.Printf("\n> ")

	input, err := ir.reader.ReadString('\n')
	if err != nil {
		return "", errors.NewInputError("Failed to read user input")
	}

	return strings.TrimSpace(strings.ToLower(input)), nil
}

// acceptSchema accepts the current schema assignment
func (ir *InteractiveReviewer) acceptSchema(item *ReviewItem) error {
	item.Status = StatusAccepted
	
	change := SchemaChange{
		EntryID:    item.EntryID,
		OldSchema:  item.CurrentSchema,
		NewSchema:  item.CurrentSchema,
		ChangeType: "accept",
	}
	ir.session.AddChange(change)

	fmt.Printf("✅ Accepted schema: %s\n\n", item.CurrentSchema)
	ir.session.NextItem()
	return nil
}

// reassignSchema allows the user to reassign the item to a different schema
func (ir *InteractiveReviewer) reassignSchema(item *ReviewItem) error {
	// Get available schemas
	schemaMap, err := ir.schemaMgr.ListSchemas()
	if err != nil {
		return errors.WrapError(
			errors.ErrCodeFileSystem,
			"Failed to list available schemas",
			err,
		)
	}

	// Flatten schema map to list
	var schemas []schema.SchemaInfo
	for _, packageSchemas := range schemaMap {
		schemas = append(schemas, packageSchemas...)
	}

	if len(schemas) == 0 {
		return errors.NewInputError(
			"No schemas available for reassignment",
			"Create a schema first using 'pudl schema add'",
		)
	}

	// Display available schemas
	fmt.Printf("\n📋 Available Schemas:\n")
	fmt.Printf("───────────────────────────────────────────────────────────────\n")
	for i, schemaInfo := range schemas {
		marker := "  "
		if schemaInfo.FullName == item.CurrentSchema {
			marker = "→ "
		}
		fmt.Printf("%s%d. %s (%s)\n", marker, i+1, schemaInfo.FullName, schemaInfo.Package)
	}
	fmt.Printf("───────────────────────────────────────────────────────────────\n")

	// Prompt for schema selection
	fmt.Printf("\nEnter schema number (1-%d) or 'cancel': ", len(schemas))
	input, err := ir.reader.ReadString('\n')
	if err != nil {
		return errors.NewInputError("Failed to read schema selection")
	}

	input = strings.TrimSpace(input)
	if input == "cancel" || input == "c" {
		fmt.Printf("❌ Schema reassignment cancelled\n\n")
		return nil
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(schemas) {
		return errors.NewInputError(
			"Invalid schema selection",
			fmt.Sprintf("Enter a number between 1 and %d", len(schemas)),
		)
	}

	selectedSchema := schemas[selection-1]

	// Validate data against selected schema
	fmt.Printf("🔍 Validating data against schema %s...\n", selectedSchema.FullName)

	if err := ir.validateDataAgainstSchema(item.Data, selectedSchema.FullName); err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)

		// Show detailed validation report
		result := ir.validationService.ValidateDataAgainstSchema(item.Data, selectedSchema.FullName)
		fmt.Printf("\n%s\n", ir.validationService.GetDetailedValidationReport(result))

		fmt.Printf("Would you like to:\n")
		fmt.Printf("  [r]etry - Choose a different schema\n")
		fmt.Printf("  [c]ontinue - Assign anyway (data will be marked as non-compliant)\n")
		fmt.Printf("  [d]etails - Show more validation details\n")
		fmt.Printf("  [b]ack - Go back to main menu\n")
		fmt.Printf("> ")

		choice, err := ir.reader.ReadString('\n')
		if err != nil {
			return err
		}

		choice = strings.TrimSpace(strings.ToLower(choice))
		switch choice {
		case "retry", "r":
			return ir.reassignSchema(item) // Recursive retry
		case "continue", "c":
			// Continue with assignment despite validation failure
			item.ValidationError = err.Error()
		case "details", "d":
			// Show additional validation details
			result := ir.validationService.ValidateDataAgainstSchema(item.Data, selectedSchema.FullName)
			fmt.Printf("\n%s\n", ir.validationService.GetDetailedValidationReport(result))
			fmt.Printf("Press Enter to continue...")
			ir.reader.ReadString('\n')
			return ir.reassignSchema(item) // Return to schema selection
		case "back", "b":
			return nil
		default:
			return nil
		}
	}

	// Update item with new schema
	oldSchema := item.CurrentSchema
	item.CurrentSchema = selectedSchema.FullName
	item.NewSchema = selectedSchema.FullName
	item.Status = StatusReassigned

	// Record the change
	change := SchemaChange{
		EntryID:    item.EntryID,
		OldSchema:  oldSchema,
		NewSchema:  selectedSchema.FullName,
		ChangeType: "reassign",
	}
	ir.session.AddChange(change)

	fmt.Printf("✅ Schema reassigned: %s → %s\n\n", oldSchema, selectedSchema.FullName)
	ir.session.NextItem()
	return nil
}

// validateDataAgainstSchema validates data against a specific schema using CUE
func (ir *InteractiveReviewer) validateDataAgainstSchema(data interface{}, schemaName string) error {
	// Use the validation service to perform real CUE validation
	result := ir.validationService.ValidateDataAgainstSchema(data, schemaName)

	if result.Valid {
		fmt.Printf("✅ Data validates against schema %s\n", schemaName)
		if result.HasFallback() {
			fmt.Printf("   Note: Data was actually assigned to %s (cascade level: %s)\n",
				result.AssignedSchema, result.CascadeLevel)
		}
		return nil
	}

	// Validation failed - create detailed error
	errorMsg := fmt.Sprintf("Validation failed for schema %s", schemaName)
	if result.ErrorMessage != "" {
		errorMsg += ": " + result.ErrorMessage
	}

	suggestions := []string{"Check the validation errors and adjust the data or schema accordingly"}
	if result.HasFallback() {
		suggestions = append(suggestions, fmt.Sprintf("Data validates against %s instead. Consider using that schema or adjusting the data.", result.AssignedSchema))
	}

	return errors.NewValidationError(schemaName, result.Errors, nil)
}

// createSchema creates a new schema from the current data item
func (ir *InteractiveReviewer) createSchema(item *ReviewItem) error {
	fmt.Printf("\n🎨 Creating new schema from data...\n")

	// Prompt for schema name suggestion
	fmt.Printf("Enter a name for this schema (or press Enter for auto-generated): ")
	input, err := ir.reader.ReadString('\n')
	if err != nil {
		return errors.NewInputError("Failed to read schema name")
	}

	suggestedName := strings.TrimSpace(input)
	if suggestedName == "" {
		suggestedName = "CustomResource"
	}

	// Create schema using the schema creator
	fmt.Printf("🔧 Generating CUE schema template...\n")
	fmt.Printf("📝 Opening editor for customization...\n")
	fmt.Printf("💡 Tip: The generated template includes field types inferred from your data\n\n")

	newSchemaName, err := ir.schemaCreator.CreateSchemaFromData(item.Data, suggestedName)
	if err != nil {
		return err
	}

	// Validate the new schema against the original data
	fmt.Printf("🔍 Validating original data against new schema...\n")
	if err := ir.validateDataAgainstSchema(item.Data, newSchemaName); err != nil {
		fmt.Printf("⚠️  Warning: Original data doesn't validate against new schema: %v\n", err)
		fmt.Printf("This is normal - you may need to adjust the schema to be more permissive.\n\n")
	}

	// Update item with new schema
	oldSchema := item.CurrentSchema
	item.CurrentSchema = newSchemaName
	item.NewSchema = newSchemaName
	item.Status = StatusCreated

	// Record the change
	change := SchemaChange{
		EntryID:    item.EntryID,
		OldSchema:  oldSchema,
		NewSchema:  newSchemaName,
		ChangeType: "create",
	}
	ir.session.AddChange(change)

	fmt.Printf("✅ Schema created and assigned: %s → %s\n\n", oldSchema, newSchemaName)
	ir.session.NextItem()
	return nil
}

// skipItem skips the current item
func (ir *InteractiveReviewer) skipItem(item *ReviewItem) error {
	item.Status = StatusSkipped
	fmt.Printf("⏭️  Skipped item\n\n")
	ir.session.NextItem()
	return nil
}

// viewFullData displays the full raw data in the system pager
func (ir *InteractiveReviewer) viewFullData(item *ReviewItem) {
	// Convert data to pretty-printed JSON
	jsonData, err := json.MarshalIndent(item.Data, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error formatting data: %v\n\n", err)
		return
	}

	// Get the pager from environment, default to "less"
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	// Create the pager command
	cmd := exec.Command(pager)
	cmd.Stdin = strings.NewReader(string(jsonData))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the pager
	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Error running pager: %v\n\n", err)
		return
	}

	fmt.Println()
}

// goBack goes back to the previous item
func (ir *InteractiveReviewer) goBack() error {
	if ir.session.PreviousItem() {
		fmt.Printf("⬅️  Going back to previous item\n\n")
		return nil
	} else {
		fmt.Printf("❌ Already at the first item\n\n")
		return nil
	}
}

// quitReview quits the review session
func (ir *InteractiveReviewer) quitReview() error {
	fmt.Printf("\n🛑 Quitting review session...\n")
	
	// Save current state
	if err := ir.sessionMgr.SaveSession(ir.session); err != nil {
		fmt.Printf("⚠️  Warning: Failed to save session: %v\n", err)
	}

	fmt.Printf("💾 Session saved. Resume with: pudl schema review --session %s\n", ir.session.SessionID)
	return fmt.Errorf("review session interrupted by user")
}

// showHelp displays help information
func (ir *InteractiveReviewer) showHelp() {
	fmt.Printf("\n📖 Schema Review Help\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("This workflow helps you review and improve schema assignments for your data.\n\n")
	fmt.Printf("Available Actions:\n")
	fmt.Printf("  accept    - Keep the current schema assignment\n")
	fmt.Printf("  reassign  - Choose a different existing schema\n")
	fmt.Printf("  create    - Create a new schema based on this data\n")
	fmt.Printf("  skip      - Skip this item (no changes)\n")
	fmt.Printf("  view      - View full raw data in system pager ($PAGER or less)\n")
	fmt.Printf("  back      - Go back to the previous item\n")
	fmt.Printf("  quit      - Save progress and exit (resumable)\n")
	fmt.Printf("  help      - Show this help message\n\n")
	fmt.Printf("Tips:\n")
	fmt.Printf("  • Use 'create' when you see a new data pattern\n")
	fmt.Printf("  • Use 'reassign' when the current schema is wrong\n")
	fmt.Printf("  • Use 'accept' when the schema is correct\n")
	fmt.Printf("  • Use 'view' to see the complete data before deciding\n")
	fmt.Printf("  • Sessions are automatically saved and resumable\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")
}

// showSummary displays the final review summary
func (ir *InteractiveReviewer) showSummary() {
	summary := ir.session.GetSummary()
	
	fmt.Printf("\n🎉 Review Session Complete!\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("  Total items:     %d\n", summary["total"])
	fmt.Printf("  ✅ Accepted:     %d\n", summary["accepted"])
	fmt.Printf("  🔄 Reassigned:   %d\n", summary["reassigned"])
	fmt.Printf("  ✨ Created:      %d\n", summary["created"])
	fmt.Printf("  ⏭️  Skipped:      %d\n", summary["skipped"])
	fmt.Printf("  ⏸️  Pending:      %d\n", summary["pending"])
	fmt.Printf("\n📝 Changes made: %d\n", len(ir.session.Changes))
	
	if len(ir.session.Changes) > 0 {
		fmt.Printf("\n🔄 Schema Changes:\n")
		for _, change := range ir.session.Changes {
			fmt.Printf("  %s: %s → %s (%s)\n", 
				change.EntryID, change.OldSchema, change.NewSchema, change.ChangeType)
		}
	}
	
	fmt.Printf("\n💡 Next steps:\n")
	fmt.Printf("  • Run 'pudl list' to see updated schema assignments\n")
	fmt.Printf("  • Use 'pudl schema commit' to version control new schemas\n")
	fmt.Printf("  • Import more data to test your new schemas\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}

// showProgressHeader displays the initial progress information
func (ir *InteractiveReviewer) showProgressHeader() {
	summary := ir.session.GetSummary()
	fmt.Printf("📊 Session Progress: %d/%d items (%.1f%%)\n",
		ir.session.CurrentIndex, len(ir.session.Items), ir.session.GetProgress())
	fmt.Printf("📋 Status: %d pending, %d reviewed\n",
		summary["pending"], summary["total"]-summary["pending"])

	if len(ir.session.Changes) > 0 {
		fmt.Printf("🔄 Changes so far: %d\n", len(ir.session.Changes))
	}
	fmt.Println()
}

// showProgressUpdate displays a brief progress update after each item
func (ir *InteractiveReviewer) showProgressUpdate() {
	if ir.session.CurrentIndex%5 == 0 || ir.session.IsComplete() {
		// Show detailed progress every 5 items or at completion
		fmt.Printf("\n📊 Progress: %d/%d items (%.1f%%) | Changes: %d\n",
			ir.session.CurrentIndex, len(ir.session.Items),
			ir.session.GetProgress(), len(ir.session.Changes))
	} else {
		// Show simple progress indicator
		fmt.Printf("📊 %d/%d (%.0f%%) ",
			ir.session.CurrentIndex, len(ir.session.Items), ir.session.GetProgress())
	}
}

// showProgressBar displays a visual progress bar
func (ir *InteractiveReviewer) showProgressBar() {
	progress := ir.session.GetProgress()
	barWidth := 40
	filled := int(progress * float64(barWidth) / 100)

	fmt.Printf("Progress: [")
	for i := 0; i < barWidth; i++ {
		if i < filled {
			fmt.Printf("█")
		} else {
			fmt.Printf("░")
		}
	}
	fmt.Printf("] %.1f%%\n", progress)
}

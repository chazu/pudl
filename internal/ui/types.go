package ui

import (
	"pudl/internal/lister"
)

// ListItem represents an item in the bubbletea list
type ListItem struct {
	Entry lister.ListEntry
	Index int
}

// FilterValue returns the string to filter on
func (i ListItem) FilterValue() string {
	// Combine multiple fields for comprehensive filtering
	return i.Entry.Proquint + " " +
		   i.Entry.ID + " " +
		   i.Entry.Schema + " " +
		   i.Entry.Origin + " " +
		   i.Entry.Format
}

// Title returns the main title for the list item
func (i ListItem) Title() string {
	collectionIndicator := ""
	if i.Entry.CollectionType != nil {
		switch *i.Entry.CollectionType {
		case "collection":
			collectionIndicator = " 📦"
		case "item":
			collectionIndicator = " 📄"
		}
	}

	return i.Entry.Proquint + " [" + i.Entry.Schema + "]" + collectionIndicator
}

// Description returns the description for the list item
func (i ListItem) Description() string {
	return "Origin: " + i.Entry.Origin + 
		   " | Format: " + i.Entry.Format + 
		   " | Records: " + formatInt(i.Entry.RecordCount) + 
		   " | Size: " + formatBytes(i.Entry.SizeBytes)
}

// formatBytes formats byte count as human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return formatInt(int(bytes)) + " B"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatFloat(float64(bytes)/float64(div), 1) + " " + "KMGTPE"[exp:exp+1] + "B"
}

// formatInt formats an integer with commas
func formatInt(n int) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	if abs < 1000 {
		return formatIntSimple(n)
	}
	return formatIntWithCommas(n)
}

// formatIntSimple formats small integers without commas
func formatIntSimple(n int) string {
	if n == 0 {
		return "0"
	}
	
	negative := n < 0
	if negative {
		n = -n
	}
	
	result := ""
	for n > 0 {
		result = string(rune('0'+(n%10))) + result
		n /= 10
	}
	
	if negative {
		result = "-" + result
	}
	
	return result
}

// formatIntWithCommas formats integers with comma separators
func formatIntWithCommas(n int) string {
	negative := n < 0
	if negative {
		n = -n
	}
	
	result := ""
	count := 0
	
	for n > 0 {
		if count > 0 && count%3 == 0 {
			result = "," + result
		}
		result = string(rune('0'+(n%10))) + result
		n /= 10
		count++
	}
	
	if negative {
		result = "-" + result
	}
	
	return result
}

// formatFloat formats a float with specified precision
func formatFloat(f float64, precision int) string {
	// Simple float formatting - for production use strconv.FormatFloat
	if precision == 1 {
		return formatFloatOneDecimal(f)
	}
	return formatFloatSimple(f)
}

// formatFloatOneDecimal formats float with one decimal place
func formatFloatOneDecimal(f float64) string {
	integer := int(f)
	decimal := int((f - float64(integer)) * 10)
	return formatIntSimple(integer) + "." + formatIntSimple(decimal)
}

// formatFloatSimple formats float as integer if no decimal part
func formatFloatSimple(f float64) string {
	return formatIntSimple(int(f))
}

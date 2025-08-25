// Simple test file with basic custom function usage
import "op"

// Test basic string operations
greeting: op.#Uppercase & {
    args: ["hello, world!"]
}

whisper: op.#Lowercase & {
    args: ["QUIET DOWN"]
}

// Test concatenation with multiple parts
fullName: op.#Concat & {
    args: ["John", " ", "Doe"]
}

// Test with different strings
title: op.#Uppercase & {
    args: ["mr."]
}

subtitle: op.#Lowercase & {
    args: ["SOFTWARE ENGINEER"]
}

// Test concatenation with more parts
address: op.#Concat & {
    args: ["123", " ", "Main", " ", "Street"]
}

// Expected results for validation
expectedGreeting: "HELLO, WORLD!"
expectedWhisper: "quiet down"
expectedFullName: "John Doe"
expectedTitle: "MR."
expectedSubtitle: "software engineer"
expectedAddress: "123 Main Street"

// Validation (these should unify successfully)
greetingCheck: greeting.result & expectedGreeting
whisperCheck: whisper.result & expectedWhisper
fullNameCheck: fullName.result & expectedFullName
titleCheck: title.result & expectedTitle
subtitleCheck: subtitle.result & expectedSubtitle
addressCheck: address.result & expectedAddress

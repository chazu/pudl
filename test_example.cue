// Test file with more complex custom function usage
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

// Test chaining results
processedGreeting: op.#Lowercase & {
    args: [greeting.result]
}

// Test with variables
message: op.#Concat & {
    args: ["Welcome, ", fullName.result, "!"]
}

// Expected results for validation
expectedGreeting: "HELLO, WORLD!"
expectedWhisper: "quiet down"
expectedFullName: "John Doe"
expectedMessage: "Welcome, John Doe!"

// Validation (these should unify successfully)
greetingCheck: greeting.result & expectedGreeting
whisperCheck: whisper.result & expectedWhisper
fullNameCheck: fullName.result & expectedFullName
messageCheck: message.result & expectedMessage

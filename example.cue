// Example of using custom functions imported via package imports
import "op"

upper: op.#Uppercase & {
	args: ["hello world"]
}

lower: op.#Lowercase & {
	args: ["HELLO WORLD"]
}

concat: op.#Concat & {
	args: ["Hello", " ", "World"]
}



uppercaseResult: upper.result // should be "HELLO WORLD"

lowercaseResult: lower.result // should be "hello world"

concatResult: concat.result // should be "Hello World"

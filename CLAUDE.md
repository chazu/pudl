Only do exactly what is asked for when writing, refactoring or debugging code. Only make changes which directly contribute to the specific task the user has given you.

When writing or refactoring code, always keep separation of concerns, readability and maintainability in mind. Keep files under 300 lines when possible, 500 lines or so at the most. Separate code into modules with single responsibilities, and make sure to avoid circular dependencies.

When debugging code, do not delete implementations and put in placeholders. If you need to debug code by removing it from the execution path, just comment it out.

When writing code, do not add placeholder implementations unless the plan you are following or the ask from the user explicitly asks for placeholders.

When completing a task, add a file to the `implog` directory summarizing the work done, including the public API implemented. Then update the plan.md to show that youve completed the work.

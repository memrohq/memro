# Changelog

## 1.1.4

- Agent mode asks before writing, editing or deleting any file, and lists the changes first.
- File paths from the model must stay inside the open folder (`..`, absolute and URI paths are refused).
- JSON that only appears in the model's reply text is no longer executed; only structured tool calls are.
- New installs start in Chat mode.
- Progress and error messages are shown as plain text.

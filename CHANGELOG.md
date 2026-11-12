# Changelog

All notable changes to Positron are listed here, newest first. Every change a user can notice adds
a line under **Unreleased** in the same commit.

## [Unreleased]

### Added
- Each agent run records what it used: tokens, how much came from the prompt cache, and the cost at API prices. Activity shows it on every run, and an agent's page shows its total for the last 7 days. Runs from before this version have no usage.

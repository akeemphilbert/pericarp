---
layout: default
title: Template
parent: Decisions
nav_exclude: true
# MADR fields — keep these three at the top.
status: proposed | accepted | deprecated | superseded by [NNNN](NNNN-title.md)
date: YYYY-MM-DD
decision-makers: who decided
---

# NNNN. Short title, stated as the decision

## Context and problem statement

Two to five sentences. What situation forced a decision, and what question had to
be answered. Name the packages, types, or contracts involved.

## Decision drivers

- A force that shaped the choice — a constraint, a quality attribute, a consumer's
  need, a failure that has already happened.
- Another.

## Considered options

1. **Option one** — one line on what it is.
2. **Option two** — one line on what it is.
3. **Option three** — one line on what it is.

## Decision outcome

Chosen option: **option N**, because … (one or two sentences tying the choice back
to the drivers).

### Consequences

- Good: what becomes easier, safer, or possible.
- Bad: what becomes harder, what is now owed, what a consumer must do differently.
- Neutral: a boundary this decision sets that a later record may revisit.

### Confirmation

How compliance is checked: a test, a linter rule, a CI job, or review. Name it.

## Pros and cons of the options

### Option one

- Good, because …
- Bad, because …

### Option two

- Good, because …
- Bad, because …

## More information

Where the decision lives in the code (paths). The issue, epic, or pull request.
Related records. If the record was written after the fact, say so and name the
source it was reconstructed from.

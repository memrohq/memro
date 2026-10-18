# 0004. License: PolyForm Noncommercial 1.0.0
Date: 2026-10-19 · Status: Accepted (supersedes a brief Apache-2.0 period)

## Context
We do not want companies to use Memro commercially for free, following the
GitNexus model. All code was written by the founder, so relicensing is clean.

## Decision
All components are source-available under PolyForm Noncommercial 1.0.0.
Commercial use requires Memro Cloud or a commercial license. Contributors
sign a CLA that allows commercial licensing.

## Alternatives considered
Apache-2.0 (max adoption), AGPL, FSL, PolyForm Shield (only blocks competing).

## Consequences
- Commits before 5cd8179 were public under Apache-2.0 and stay so.
- We must not call it "open source"; use "source-available".
- SDKs are also noncommercial: Memro Cloud terms must grant customers the
  right to use the SDKs/MCP with Memro Cloud.

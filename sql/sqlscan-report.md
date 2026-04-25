# API SQL Injection Report

**Target:** example-public-api  
**Base URL:** https://api.example.com  
**Generated:** 2026-04-20 13:19:30 UTC

## Summary

| Endpoints | Requests | Findings | High | Medium | Low |
| --- | ---: | ---: | ---: | ---: | ---: |
| 2 | 2 | 0 | 0 | 0 | 0 |

## Endpoint Baselines

| Endpoint | Method | Path | Status | Duration (ms) | Notes |
| --- | --- | --- | ---: | ---: | --- |
| search-users | GET | `/users/search` | 0 | 0 | Skipped: Get "https://api.example.com/users/search?q=alice": dial tcp: lookup api.example.com: no such host |
| login | POST | `/auth/login` | 0 | 0 | Skipped: Post "https://api.example.com/auth/login": dial tcp: lookup api.example.com: no such host |

## Findings

No clear SQL injection indicators were detected with the current payload set.

## Hardening Suggestions

- Keep using parameterized queries, strict input validation, and generic error messages for database-backed endpoints.


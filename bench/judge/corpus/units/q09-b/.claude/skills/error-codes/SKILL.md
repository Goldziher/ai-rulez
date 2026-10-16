---
name: error-codes
description: "Explains an Acme API error code and the fix. Use when the user pastes an E-code from an API response."
---

# Error codes

Find the code the user pasted in the table, quote its meaning and give the action.

| Code | Meaning | Action |
|------|---------|--------|
| E1000 | Invalid session token | Re-authenticate |
| E1001 | Expired session | Refresh the session |
| E1002 | Missing API key | Add the header |
| E1003 | Rate limit exceeded | Retry after the delay |
| E1004 | Payload too large | Split the request |
| E1005 | Unsupported media type | Send JSON |
| E1006 | Unknown tenant | Check the tenant slug |
| E1007 | Tenant suspended | Contact support |
| E1008 | Invalid cursor | Restart pagination |
| E1009 | Duplicate request | Use a new idempotency key |
| E1010 | Validation failed | Fix the listed fields |
| E1011 | Unknown field | Remove the field |
| E1012 | Resource locked | Retry later |
| E1013 | Resource not found | Check the identifier |
| E1014 | Conflict on update | Fetch and retry |
| E1015 | Upstream timeout | Retry with backoff |
| E1016 | Upstream unavailable | Retry later |
| E1017 | Quota exceeded | Raise the quota |
| E1018 | Invalid signature | Recompute the signature |
| E1019 | Clock skew too large | Sync the clock |
| E1020 | Bad date range | Use ISO dates |
| E1021 | Unsupported locale | Use a listed locale |
| E1022 | Webhook disabled | Re-enable the webhook |
| E1023 | Webhook URL unreachable | Fix the URL |
| E1024 | Export too large | Narrow the filter |
| E1025 | Export expired | Request a new export |
| E1026 | Import row invalid | Fix the row |
| E1027 | Import header missing | Add the header row |
| E1028 | Import encoding invalid | Use UTF-8 |
| E1029 | Report not ready | Poll again |
| E1030 | Report failed | Retry the report |
| E1031 | Role missing | Grant the role |
| E1032 | Scope missing | Request the scope |
| E1033 | Password too weak | Pick a longer one |
| E1034 | Account locked | Wait or reset |
| E1035 | Email unverified | Verify the email |
| E1036 | Invalid coupon | Check the code |
| E1037 | Coupon expired | Use a current coupon |
| E1038 | Card declined | Use another card |
| E1039 | Currency mismatch | Use the account currency |
| E1040 | Refund exceeds charge | Lower the amount |
| E1041 | Charge already refunded | No action |
| E1042 | Plan not found | Choose a listed plan |
| E1043 | Plan downgrade blocked | Cancel add-ons first |
| E1044 | Seat limit reached | Buy seats |

If the code is not in the table, say so and ask for the full response body.

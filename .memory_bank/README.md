# Memory bank for AI agents

## Mocks

For mocking we use `go.uber.org/mock`.

Mock packages have `mock_` prefix.
For example all mocks for [`dal`](../dal) are in [`mock_dal`](../mock_dal) package.

When you need to mock use mocks provided. If no required mock found created it using `mockgen` by `go.uber.org/mock` -
examples of usage are in [`../generate_mocks.sh`](../generate_mocks.sh). 
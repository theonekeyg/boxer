# Uploading boxer-sdk to PyPI

## Requirements

- [uv](https://docs.astral.sh/uv/) installed
- PyPI account with an API token

## Steps

```bash
cd packages/sdk/python

# Build distribution artifacts (creates dist/)
uv build

# Upload to PyPI
UV_PUBLISH_TOKEN=pypi-your-token-here uv publish
```

## TestPyPI (optional, to verify before publishing)

```bash
uv publish --publish-url https://test.pypi.org/legacy/ --token pypi-your-test-token
```

## Notes

- Bump `version` in `pyproject.toml` before each release — PyPI does not allow overwriting existing versions.
- API tokens can be created at: Account Settings → API tokens on pypi.org.

# wtf capabilities

Describe WTF's versioned machine-readable contracts without inferring support from
the release version.

## Usage

```bash
wtf capabilities --json
```

The result reports schema versions, supported VCS backends, resource kinds, and
doctor checks. Automation such as Agent Bridge should validate this response before
calling structured workspace operations. WTF remains independently usable and never
calls or stores Agent Bridge data.

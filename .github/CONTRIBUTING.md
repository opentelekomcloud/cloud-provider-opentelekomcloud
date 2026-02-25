# Contributing to Cloud Provider OpenTelekomCloud

- [3 ways to get involved](#3-ways-to-get-involved)
- [Getting started](#getting-started)
- [Tests](#tests)
- [Style guide](#basic-style-guide)

## 3 ways to get involved

There are three main ways you can get involved in our open-source project, and
each is described briefly below.

### 1. Fixing bugs

If you want to start fixing open bugs, we'd really appreciate that! Bug fixing
is central to any project. The best way to get started is by heading to our
[bug tracker](https://github.com/opentelekomcloud/cloud-provider-opentelekomcloud/issues) and finding open
bugs that you think nobody is working on. It might be useful to comment on the
thread to see the current state of the issue and if anybody has made any
breakthroughs on it so far.

### 2. Improving documentation

If you feel that a certain section could be improved - whether it's to clarify
ambiguity, correct a technical mistake, or to fix a grammatical error - please
feel entitled to do so! We welcome doc pull requests with the same enthusiasm
as any other contribution!

### 3. Working on a new feature

If you've found something we've left out, definitely feel free to start work on
introducing that feature. It's always useful to open an issue or submit a pull
request early on to indicate your intent to a core contributor - this enables
quick/early feedback and can help steer you in the right direction by avoiding
known issues. It might also help you avoid losing time implementing something
that might not ever work. One tip is to prefix your Pull Request issue title
with [wip] - then people know it's a work in progress.

We ask that you do not submit a feature that you have not spent time researching
and testing first-hand in an actual Open Telekom Cloud environment. While we appreciate
the contribution, submitting code which you are unfamiliar with is a risk to the
users who will ultimately use it.

Please do not hesitate to ask questions or request clarification. Your
contribution is very much appreciated and we are happy to work with you to get
it merged.

## Getting Started

As a contributor you will need to setup your workspace in a slightly different
way than just downloading it. Here are the basic instructions:

1. Fork the `opentelekomcloud/cloud-provider-opentelekomcloud` repository on GitHub.

2. Clone your fork locally:

   ```bash
   git clone git@github.com:<my_username>/cloud-provider-opentelekomcloud.git
   cd cloud-provider-opentelekomcloud
   ```

3. Add the upstream remote:

   ```bash
   git remote add upstream git@github.com:opentelekomcloud/cloud-provider-opentelekomcloud.git
   ```

4. Create a new feature branch from `master`:

   ```bash
   git checkout -b my-new-feature
   ```

5. Make your changes, then build and test:

   ```bash
   make build
   make test
   ```

6. Run linters before committing:

   ```bash
   make lint
   make vet
   ```

7. Commit and push your changes:

   ```bash
   git add path/to/changed/file.go
   git commit
   git push origin my-new-feature
   ```

8. Submit a [Pull Request](https://help.github.com/articles/creating-a-pull-request/)
   against the `master` branch.

> Further information about using Git can be found [here](https://git-scm.com/book/en/v2).

Happy Hacking!

## Tests

When working on a new or existing feature, testing will be the backbone of your
work since it helps uncover and prevent regressions in the codebase.

### Unit tests

Unit tests are the fine-grained tests that establish and ensure the behavior
of individual units of functionality. They live alongside the source code in
`_test.go` files.

To run all unit tests:

```bash
make test
```

### Running tests

To run all tests:

```bash
make test
```

To run tests for a particular sub-package:

```bash
go test -v ./pkg/opentelekomcloud/...
```

## Basic style guide

- Follow standard Go conventions (`go fmt`, `go vet`).
- All code must pass `golangci-lint` checks (see `.golangci.yaml` for configuration).
- Use `make fmt` to format code before committing.
- Keep pull requests focused - one feature or fix per PR.
- Write meaningful commit messages.

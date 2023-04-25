# Contributing Guide

We are happy you are interested in contributing.
Before contributing, there are some things you should know.

1. If you are creating a new implementation please consider putting that in its
   own repository. We are happy to link to that from this repository.
2. This repository follows [semantic versioning](https://semver.org/). All public
   APIs (e.g. function names and arguments) can't change without incrementing
   the major version. Something that will be rare, if ever.
3. If you want to make some major changes to the reference implementations
   consider making another implementation in a separate repository. It's ok. Let
   us know, and we'll provide a link to it in the README.

## Pull Requests

All changes to this repository happen through pull requests on GitHub. If you
have a proposed change it will need to be in the form of a pull request.

When creating the pull request please provide a use case and justification for
the change if it is more than a minor change.
Changes to the functionality need a good justification.

## Developers Certificate of Origin

All changes to this repository need sign-off for the [Developers Certificate of Origin](https://developercertificate.org/).
You can read it below. After that are some details on how to sign you work.

Note, if you created a pull request and missed signing a commit CI will catch it
and provide directions on how correct the issue.

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Then you just add a line to every git commit message:

    Signed-off-by: Joe Smith <joe.smith@example.com>

Use your real name (_sorry, no pseudonyms or anonymous contributions_).

If you set your `user.name` and `user.email` git configs, you can sign your commit automatically
with `git commit -s`.

Note: If your git config information is set properly then viewing the `git log` information for your
commit will look something like this:

```
Author: Joe Smith <joe.smith@example.com>
Date:   Thu Feb 2 11:41:15 2018 -0800

    Update README

    Signed-off-by: Joe Smith <joe.smith@example.com>
```

Notice the `Author` and `Signed-off-by` lines match.
If they don't your PR will be rejected by the automated DCO check.

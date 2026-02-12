> This file guides agents on how to work on this code base when needed for boiler plat tasks.
> There are few things agents needs to know before producing output.

- we prioritize maintainability rather than just fix or just work.
- we use a build file called make for builds, tests and local development
- our dev environment is goland so things like govet, ci lint and fmt are handled at IDE level rather than in CI
- For our tests, we don't enforce style, we chose the right style based on what we want to test.
- we use table driven tests where appropriate, subtests where its needed and normal tests where useful

#### To Agents:

> Important for all agents to follow:   
- if you edit a file, always make sure the extensive header comment on the file matches what the whole file is about
- always make sure extensive comments on every function has no weird syntax and is extensive enough to onboard a junior dev
- don't leave a function without extensive comment on what it does to make on onboarding easy
- for every part of a complicated code add comments
- commits should be extensive and capture changes in small isolations
- for branch naming - feature/name or fix/name depending on task
- build system to use is always make at repo root
- service default to white box testing
- http defaults to blackbox testing


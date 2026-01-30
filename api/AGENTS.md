> This file guides agents on how to work on this code base when needed for boiler plat tasks.
> There are few things agents needs to know before producing output.

- we prioritize maintainability rather than just fix or just work.
- This default is blackbox testing and won't test private packages for simplicity
- we use a build file called make for builds, tests and local development
- our dev environment is goland so things like govet, ci lint and fmt are handled at IDE level rather than in CI
- In everything we do, we find way to write less code while still prioritizing simplicity and clarity
- For our tests, we don't enforce style, we chose the right style based on what we want to test.
- we use table driven tests where appropriate, subtests where its needed and normal tests where useful

#### To Agents:

> Important for all agents to follow:

We won't restrict you to not limit your creativity but our values should be put into consideration so we can
learn from each other

- don't be shy to point out where implementation is not right or not secure be default
- always follow production grade practices because code here has to be reliable than ever, we host apps not bugs.
- always follow the best implementation that will produce best results.
- always spot where there are inconsistencies in codebase and provide the best ways to resolve them.
- the makefile is your friend.
- comments are important part to make complex code understandable. Official go code base teaches us this.
- so write comments to explain complex implementation and comments at the beginning of file.
- if you work directly in the file check if the comments at the beginning of the file and other places make sense after
  your edits
- tests are your friend, if you work directly in the codebase always test your implementation.
- so the goDoc comments on every function must be present 12 lines max and always use function name to start as expected
  by go
- always optimize for clarity, simplicity, maintainability
- for the header files comment the package as to prefix it as required by GoDoc
- gh cli is available to you to pull additional context so mostly this codebase is issue/ feature branch driven to keep
  context histories

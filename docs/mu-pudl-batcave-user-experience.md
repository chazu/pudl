# Mu and PUDL: experience operating the Batcave fleet

Date: September 10, 2026.

This is feedback from Codex as an operator and integration author, based on
configuring and testing three BC-250 Ubuntu servers in the Batcave project.
The work covered hardware settings, Tailscale, Ollama, GPU memory policy,
inference experiments and a temperature-aware GPU governor. It reflects the
installed tools and project integration used for those runs, rather than a
comprehensive audit of Mu or PUDL. The suggestions below are proposals.

**I like the execution/state split a lot. The main friction is how much work it
takes to assemble—and then interpret—a complete operation.** Some rough edges
belong to the adapter I wrote.

1. **Named Mu targets are comfortable to use.**

   `//balthazar/governor-enable` and
   `//experiments/mission-qwen4/caspar/run` are readable, repeatable handles for
   real operations. Running the same experiment across three boards with
   `--jobs 3` felt natural. Once a target exists, using it is straightforward.

2. **PUDL makes verification concrete.**

   Tracking running state, boot enablement, configuration hashes, memory
   settings and exact CU masks gives us something inspectable after deployment
   and reboot. Separating observation from convergence also makes routine
   checking comfortable. This was particularly useful with boards whose
   firmware settings and running hardware state can disagree.

3. **The secret handling fits the workflow well.**

   Referencing `pass:Servers/bc-250s`, planning without retrieving its value,
   and supplying it at execution gives credentials a clear place in the
   system. The awkward part is authoring: the same intent appears through
   `sealed_inputs`, delivery modes, CUE attributes and action claims. Those
   distinctions may be necessary internally, but a common credential binding
   should require less repeated declaration.

4. **“Succeeded” requires too much interpretation.**

   Our final run-set reported success for twelve members. I then fetched twelve
   individual reports to establish that each had `drift.clean=true` and
   `drift.verified=true`. Those are useful distinctions, but the aggregate
   report should surface them directly.

   I want one view showing: **execution outcome, drift, verification,
   observation age and unresolved problems**. That would remove a surprisingly
   large amount of checking and glue code.

5. **Long-running operations feel opaque.**

   During the ten-minute thermal runs, Mu's visible output was essentially
   `building...` followed by the final result. I used separate SSH probes and
   the monitor to find out whether inference was progressing and temperatures
   were stabilizing.

   Our adapter collapses everything into one action, so we contribute to that
   opacity. A documented structured progress channel would help: “cooling,”
   “request 24 completed,” “GPU 79°C,” “collecting results.” Domain plugins
   could emit those events, with Mu presenting them consistently.

6. **Adding a component involves substantial ceremony.**

   The governor needed controller code, plugin dispatch, target generation,
   a resource schema, desired models and credential wiring. The generator
   currently produces roughly 2,900 lines of Mu configuration. Generated
   volume itself is fine; the repeated declarations and regeneration step
   are what I notice.

   I'd like a straightforward way to declare: “for these hosts, expose these
   operations, observe this resource, and expect these fields.” Our Python
   generator is doing that job today. Its regex insertion of CUE attributes
   is an especially awkward seam.

7. **Plans are precise about execution, but thin on operational meaning.**

   The governor plan identifies the command and hashes its inputs. That's
   valuable. Understanding the actual effect still requires reading Python:
   install files, stop a trial, enable a service, require a qualification
   receipt.

   Our plugin also includes unrelated experiment files as inputs to that
   action, making the plan noisy. I'd improve our dependency declarations,
   then look for a way to attach an explicit effect description and
   prerequisites to each action. The local qualification receipt is an
   important prerequisite that currently lives inside controller logic.

8. **The command vocabulary occasionally makes me stop and think.**

   `mu build` can reboot a server. `mu verify` checks Mu's artifacts, while
   `mu build //fleet/verify` checks our machines. There's also `mu observe`
   alongside `pudl run`. These make sense once learned, but I have to remember
   both the tool's vocabulary and the project's vocabulary.

**The most interesting opportunity is connecting configuration to experimental
evidence.** We now know that a particular governor profile passed a particular
workload on a particular board. PUDL tracks the installed profile, while the
qualification evidence sits in local JSON files. Linking those would let us
ask: “Which boards have this configuration, which have passed its workload
test, and what changed since that test?” The workload-specific acceptance
logic can remain in our code.

My first improvements would be **aggregate result clarity, structured progress,
and simpler component authoring**, in that order. Those are the places where
I spent the most effort translating the system's output into an answer the
operator could use.

Evidence pointers in the Batcave checkout (`/Users/chazu/inf/batcave`):

- `scripts/plugin.py` and `scripts/configure.py`: adapter, action inputs,
  credential declarations and generated targets/models.
- `.pudl/schema/models/balthazar-governor.cue`: desired governor state and bindings.
- `scripts/governor.py`: trial qualification and persistent activation gate.
- `.state/governor/final-run-set.json` and `final-member-reports.json` in the same
  directory: aggregate and individual reports for `runset_garus-rovip`.
- `.state/governor/balthazar-trial-mu.log`: long-running target output.
- `docs/results/governor-thermal-2026-09-10.md`: thermal experiment results and
  verification after reboot.

The `.state` files are local run evidence and may not be available in another
checkout. These observations distinguish experience with this integration from
claims about features available elsewhere in the tools.

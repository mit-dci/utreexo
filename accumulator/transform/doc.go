/*

Transform package is used to calculate where each leaves should move to during
a batch deletion. This significantly increases performance during deletions
compared to deleting leaves one by one. No actual deletions happen in a tree
during this phase.

idea for transform
get rid of stash, and use swaps instead.
Where you would encounter stashing, here's what to do:
stash in place: It's OK, that doesn't even count as a stash
stash to sibling: Also OK, go for it.  The sib must have been deleted, right?

stash elsewhere: Only swap to the LSB of destination (sibling).  If LSB of
destination is same as current LSB, don't move.  You will get there later.
When you do this, you still flag the parent as "deleted" even though it's still
half-there.

Maybe modify removeTransform to do this; that might make leaftransform easier

RemTrans2 is the main workhorse for the transform step. We'll go through an
example with a tree like:

		06
		|-------\
		04      05
		|---\   |---\
		00  01  02  03

	1) DelRoot:
		The roots are deleted before progressing. This is because
		they will be incorrect as the lower hashes will change.
		The tree would look like:


		|-------\
		04      05
		|---\   |---\
		00  01  02  03

		Note that the 06 hash is removed.

	2) Twin:
		If both tree leaves are being deleted, mark those for deletion.


	3) Swap:
*/
package transform

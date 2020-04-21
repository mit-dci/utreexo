Transform is used to calculate where each leaves should move to during a
batch deletion. This significantly increases performance during deletions
compared to deleting leaves one by one. No actual deletions happen in a tree
during this phase.

RemTrans() is the main workhorse for the transform step.


The deletions are marked from the bottom to the top row. We go through row
with the steps outlined below.

1) DelRoot:
The roots are deleted before progressing. This is because they will be incorrect
as the lower hashes will change. 

Here we have a tree of row 2. When RemTrans is on row 2, 06 will be
deleted. On all the other rows, nothing will happen.

	row 2:	06
		|-------\
	row 1:	04......05
		|---\...|---\
	row 0:	00..01..02..03

In a tree like below the deletion will happen on row 1 as well since position
10 is a root.

	em
	|---------------\
	11..............em
	|-------\.......|-------\
	08......09......10......em
	|---\...|---\...|---\...|---\
	00..01..02..03..04..05..em..em

2) Twin:
Find siblings that are being deleted and mark those for deletion. The parent
is also marked for deletion.

Note that 02 and 03 are being deleted so we can mark those. The parent, 09,
is also marked for deletion.

	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......**......10......11
	|---\...|---\...|---\...|---\
	00..01..**..**..04..05..06..07


3) Swap:
When a leaf's sibling and one of its cousin is deleted, the remaining cousin
becomes its sibling.

	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......**
	|---\...|---\...|---\...|---\
	00..01..02..03..04..**..**..07

Here, 04's sibling (05) and one of its cousins(06) got deleted. 07 would move
to 05's position. 11 is also marked for deletion as 06 is deleted and 07 is
marked to move.

4) Root:
When a leaf's sibling is deleted, either:

1. Move a root to the sibling.
2. If there is no root in that row, then that leaf becomes a root.

Here demonstrates (1). 05 is deleted and 06 is a root so it moves to 05's
position.

	em
	|---------------\
	12..............em
	|-------\.......|-------\
	08......09......10......em
	|---\...|---\...|---\...|---\
	00..01..02..03..04..**..06..em

Here demonstrates (2). 05 and 06 are deleted. 10 is marked for deletion and
04 becomes its own root.

	em
	|---------------\
	12..............em
	|-------\.......|-------\
	08......09......**......em
	|---\...|---\...|---\...|---\
	00..01..02..03..04..**..**..em


5) Climb:
Move to the row above from the deleted leaves If there is no more rows to
climb, the transform is finished. No actual markings happen in this phase.

TODO add example tree deletion.

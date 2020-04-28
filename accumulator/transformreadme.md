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

Here we have a tree of height 2. When RemTrans is on row 2, 06 will be
deleted. On all the other rows, nothing will happen.

	06
	|-------\
	04......05
	|---\...|---\
	00..01..02..03

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
Find siblings that are being deleted and mark those for deletion.

3) Swap:
When a leaf's sibling and one of its cousin is deleted, the remaining cousin
becomes its sibling.

4) Root:
When a leaf's sibling is deleted, move a root to the sibling. If there is no
root in that row, then that leaf becomes a root.

5) Climb:
Move to the row above from the deleted leaves. If there is no more rows to
climb, the transform is finished.

	em
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	em..em..02..03..04..05..06..07

We'll go through an example with a tree like:
	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	00..01..02..03..04..05..06..07

	14
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	00..01..02..03..04..05..06..07

	em
	|---------------\
	12..............13
	|-------\.......|-------\
	08......09......10......11
	|---\...|---\...|---\...|---\
	em..em..02..03..04..05..06..07

Here 00 and 01 was marked for deletion.


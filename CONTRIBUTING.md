## RULES TO FOLLOW WHEN MERGING CODE

- Redundant Data Normalization is not allowed if data as been verified once, and we are sure it's not going to change.
this means that the use of irrelevant error checks when we are 100% certain this won't throw an error are not allowed. 
they just cause unnecessary boilerplate and make lines longer. same also goes for string checks and other places validation 
is necessary
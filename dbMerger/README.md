# DB merger tool

- These types of tool are able to merge level-DBs directories by creating a new one containing all data.

## The tools
### generalDBMerger tool

- This tool is a general purpose tool able to copy the data from any number of level-DBs into a new one.
It can contain any type of keys and values of any length and the processing & override rules are the following:
1. the tool copies the first source DB at the OS level (using provided raw-data copy functionality)
2. it then opens, in order, the next DBs provided as source and iterates over all existing keys and values, 
storing them in the destination DB.

How to use:

```
cd cmd/generalDBManager
go build
```

after the compilation of the binary, the merge can be made by calling the binary with the following parameters:

```
mkdir destdb
./generalDBManager -dest=./destdb -sources=./src1/db,./src2/db,./src3/db
```

for full flags list, launch the binary with the following parameter

```
./generalDBManager -h
```

### trieMerger tool

< to be implemented >

## Audience

This tool should be as generic as possible, and it shouldn't have any custom code related to Elrond instances.

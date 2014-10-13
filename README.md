#DhtHell

DhtHell is a program used to test out ipfs's DHT and other features, you can run it as a simple command line, and run all the commands you want yourself, or you can give it a config/command file to set up the initial network how you like, and maybe even run some commands for you.


##Command File Syntax
The first line of the command file specifies the number of nodes to create.
Following that line, and until a line containing "--" is reached, you may specify bootstrapping orders with the following syntax: `[range]->[range]` where range is a number or set of numbers in one of the following forms: `by itself: 'X', single number list: '[X]' or full inclusive range: '[X-Y]'`

For Example:

	[1-4]->0

Specifies that nodes 1 through 4 should use node 0 as a boostrapping node.

Following the break sequence ("--") you may specify commands to run with the following syntax:

	node# command args

For example:

	4 put test hello

Would tell node 4 to run a put of the value "hello" for the key "test"

The Sequence "==" signals to switch input over to standard in, allowing the user to manually enter commands

## Commands

Put:
	Args: key, val

Get:
	Args: key

Store:
	Args: key, val

Provide:
	Args: key

Diag:
	Args: none!

FindProv:
	Args: key


## Example

	25
	[1-3]->0
	[4-9]->1
	[10-20]->2
	[21-24]->3
	--
	6 put key val
	23 get key
	2 get key
	==

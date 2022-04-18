# utreexo

utreexo is a novel hash-based dynamic accumulator, which allows the millions of unspent outputs to be represented under a kilobyte -- small enough to be written on a sheet of paper. There is no trusted setup or loss of security; instead, the burden of keeping track of funds is shifted to the owner of those funds.

Check out the ePrint paper here: https://eprint.iacr.org/2019/611

Currently, transactions specify inputs and outputs, and verifying an input requires you to know the whole state of the system. With utreexo, the holder of funds maintains a proof that the funds exist and provides that proof at spending time to the other nodes. These proofs represent the utreexo model’s main downside; they present an additional data transmission overhead that allows a much smaller state.

utreexo pushes the costs of maintaining the network to the right place: an exchange creating millions of transactions may need to maintain millions of proofs, while a personal account with only a few unspent outputs will only need to maintain a few kilobytes of proof data. utreexo also provides a long-term scalability solution as the accumulator size grows very slowly with the increasing size of the underlying set (the accumulator size is logarithmic with the set size)

# Documentation

- [Installation](https://github.com/mit-dci/utreexo/blob/master/docs/installation.md)
- [Style Guidelines](https://github.com/mit-dci/utreexo/blob/master/docs/style.md)
- [Contributing Guidelines](https://github.com/mit-dci/utreexo/blob/master/docs/contributing.md)
- [Contact](https://github.com/mit-dci/utreexo/blob/master/docs/contact.md)

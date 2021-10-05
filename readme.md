# utreexo

![utreexo cover image](/docs/assets/utreexo.png)

utreexo is a novel hash-based dynamic accumulator, which allows the millions of unspent outputs to be represented under a kilobyte -- small enough to be written on a sheet of paper. There is no trusted setup or loss of security; instead, the burden of keeping track of funds is shifted to the owner of those funds.

Check out the ePrint paper here: https://eprint.iacr.org/2019/611

Currently, transactions specify inputs and outputs, and verifying an input requires you to know the whole state of the system. With utreexo, the holder of funds maintains a proof that the funds exist and provides that proof at spending time to the other nodes. These proofs represent the utreexo model’s main downside; they present an additional data transmission overhead that allows a much smaller state.

utreexo pushes the costs of maintaining the network to the right place: an exchange creating millions of transactions may need to maintain millions of proofs, while a personal account with only a few unspent outputs will only need to maintain a few kilobytes of proof data. utreexo also provides a long-term scalability solution as the accumulator size grows very slowly with the increasing size of the underlying set (the accumulator size is logarithmic with the set size)

## Development Process

At the moment, this repository holds the code for the accumulator and proof-of-concept implementations for the compact state node and the bridge node. The accumulator package is currently used in [mit-dci/utcd](https://github.com/mit-dci/utcd) in an alternative full node bitcoin implementation written in Go.

The c++ implementation of utreexo is at [mit-dci/libutreexo](https://github.com/mit-dci/libutreexo) and the package is currently being used in [dergoegge/bitcoin](https://github.com/dergoegge/bitcoin) in an full node bitcoin-core implementation.

## Documentation

It is located in the [docs](https://github.com/mit-dci/utreexo/docs) folder.

## IRC

Currently under active development. If you're interested and have questions, checkout #utreexo on irc.libera.chat.

Logs for libera are [here](https://gnusha.org/utreexo/)

## Contributions

- Feel Free to Open a PR/Issue/Discussion for any features/bug(s)/question(s).
- Make sure you follow the contributing guidelines [here]()!

## Resources

### Blogs

- https://dci.mit.edu/utreexo
- [Utreexo demonstration release](https://medium.com/mit-media-lab-digital-currency-initiative/utreexo-demonstration-release-a0d87506fd70)
- [Utreexo demo release 0.2](https://medium.com/mit-media-lab-digital-currency-initiative/utreexo-demo-release-0-2-ac40a1223a38)
- [Utreexo — A scaling solution](https://medium.com/@kcalvinalvinn/eli5-utreexo-a-scaling-solution-9531aee3d7ba)

### Videos/Podcasts

- [MIT Bitcoin Expo 2019 - Utreexo: Reducing Bitcoin Nodes to 1 Kilobyte](https://www.youtube.com/watch?v=edRun-6ubCc)
- [Tadge Dryja on Scaling Bitcoin With Utreexo](https://www.youtube.com/watch?v=2Kg1Cij3w20)
- [#1 Tadge Dryja, Digital Currency Initiative: uTreeXO and Bootstrapping Bitcoin Upgrades](https://www.youtube.com/watch?v=-MlKZ_bFLNk)

# Kraken WebSocket order book listener

* This repo implements listening to incremental updates of the order book.
* Order book integrity check is done in `verifyOrderBookChecksum` function.
* Order book sorting is time-optimized to scale in logarithmic fashion together with the size of the dataset.

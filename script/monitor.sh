#! /bin/bash
while true
do
	monitor=`ps -ef | grep solana_arbitrage | grep -v grep | wc -l`
	if [ $monitor -eq 0 ]; then
		echo "Program is not running, restart solana_arbitrage"
		./solana_arbitrage ./.. &
	else
		echo "Program is running"
	fi
	sleep 30
done

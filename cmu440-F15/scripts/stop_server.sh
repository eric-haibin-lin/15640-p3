while read line
do
    kill -9 $line
done <server_log.txt
rm server_log.txt
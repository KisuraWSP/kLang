```
How to Make Your Language Beat Python

If your custom programming language is currently flushing the output stream to the console on every single number (or every newline), it is wasting massive amounts of CPU cycles waiting on the operating system.

To make your language faster than Python, check your language compiler or interpreter implementation for these two areas:
    Implement Block Buffering: 
        Ensure your print library buffers output text in memory (e.g., a 4KB or 8KB buffer) and only flushes it to standard output when the buffer is completely full, rather than on every newline.
    Optimize Integer-to-String Conversion: 
        Converting binary integers to ASCII characters is a heavy bottleneck in loops. If your language uses a standard, unoptimized modulo loop (% 10 and / 10), switching to a fast algorithm like the Jeaiat or digits-of-two method can cut your conversion time in half.
```

# RIGHT NOW
```shell
9.7611s (python took this much time to print from 0 to 10 million)
14.377841s (our programming language took this much time to print from 0 to 10 million)
```
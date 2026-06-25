import time
import sys

def test_standard_print(limit):
    print(f"--- Starting Standard Print (0 to {limit}) ---")
    start_time = time.perf_counter()
    
    for i in range(limit + 1):
        print(i)
        
    end_time = time.perf_counter()
    duration = end_time - start_time
    return duration

def test_optimized_print(limit):
    print(f"--- Starting Optimized Batch Print (0 to {limit}) ---")
    start_time = time.perf_counter()
    
    # Converts all numbers to strings, joins with newlines, and writes at once
    sys.stdout.write('\n'.join(map(str, range(limit + 1))) + '\n')
    
    end_time = time.perf_counter()
    duration = end_time - start_time
    return duration

if __name__ == "__main__":
    # WARNING: 10,000,000 will freeze slow IDEs. 
    # We use 100,000 first so you can safely test your speed.
    TEST_LIMIT = 10000000
    
    print("Testing with 100,000 numbers to check terminal speed safely.\n")
    
    # Run optimized test first (safe and fast)
    opt_time = test_optimized_print(TEST_LIMIT)
    
    # Run standard test
    std_time = test_standard_print(TEST_LIMIT)
    
    print("\n=== RESULTS ===")
    print(f"Optimized Batch Print: {opt_time:.4f} seconds")
    print(f"Standard Loop Print:    {std_time:.4f} seconds")
    print(f"Batching was {std_time / opt_time:.1f}x faster!")

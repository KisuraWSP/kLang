# 19 June 12:45
```shell
// test files loc
(base) kisuraw.s.p@Defers-Mackbook kLang % cloc --include-lang=Go --match-f=".*_test\.go$" .
       9 text files.
       9 unique files.                              
       1 file ignored.

github.com/AlDanial/cloc v 2.08  T=0.02 s (387.7 files/s, 366793.7 lines/s)
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                               9           1056             10           7448
-------------------------------------------------------------------------------
SUM:                             9           1056             10           7448
-------------------------------------------------------------------------------

// main codebase loc
(base) kisuraw.s.p@Defers-Mackbook kLang % cloc --include-lang=Go --not-match-f=".*_test\.go$" .
     271 text files.
     122 unique files.                                          
     256 files ignored.

github.com/AlDanial/cloc v 2.08  T=0.08 s (239.4 files/s, 230326.6 lines/s)
-------------------------------------------------------------------------------
Language                     files          blank        comment           code
-------------------------------------------------------------------------------
Go                              20           1303              8          17933
-------------------------------------------------------------------------------
SUM:                            20           1303              8          17933
-------------------------------------------------------------------------------
```
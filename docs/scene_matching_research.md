# **Architectural Paradigms for Semantic Media Reconciliation and Metadata Extraction in Go**

## **1\. Introduction: The Semantic Gap in Automated Media Management**

The core engineering challenge in building a media downloading application akin to the \*arr stack (Sonarr, Radarr, Lidarr) lies in resolving the fundamental dissonance between human intent and technical reality. This dissonance creates a "Semantic Gap".1 On one side of this gap exists the "Human Title"—the linguistic construct used by a user to request content, characterized by ambiguity, colloquialism, and brevity (e.g., "Back to the Future 2"). On the other side exists the "Scene Release"—a rigid, hyper-structured, and technically encoded string designed not for human readability, but for machine sorting and version control within the Warez Scene (e.g., Back.to.the.Future.Part.II.1989.2160p.UHD.BluRay.x265-TERMINAL).

Reconciling these two ontologies requires a system capable of translation, normalization, and semantic inference. Historically, this problem has been addressed through deterministic means: massive regular expression (regex) libraries, static lookup tables, and community-maintained mapping databases like TheTVDB or TMDB.3 While computationally efficient, these deterministic systems are inherently brittle. They fail when confronted with non-standard naming conventions, complex linguistic variations (e.g., "Fast Five" vs. "Fast & Furious 5"), or the "Absolute Numbering" schemes prevalent in anime releases.5

This report proposes a next-generation architectural framework for media parsing and matching in Go, moving beyond purely deterministic logic toward a **Hybrid Probabilistic-Deterministic Pipeline**. By integrating Small Language Models (SLMs) and vector-based semantic search directly into the Go application binary, developers can achieve a level of "cognitive parsing" previously unattainable without heavy external dependencies. This approach leverages the high-performance concurrency of Go for data ingestion while utilizing embedded C++ bindings (via CGO) or pure Go implementations for AI inference, creating a system that is both robust and elegant.

### **1.1 The Ontology of the Problem**

To design a robust solution, one must first deconstruct the distinct data structures involved in the reconciliation process. The mismatch is not merely syntactic; it is ontological.

The **Human Query** is an expression of intent. It is unstructured string data requiring normalization, expansion, and semantic disambiguation. When a user searches for "The Office," the system must determine if they intend the 2001 UK series or the 2005 US series. This requires context, often derived from year or country metadata, which may be missing from the query itself.7

The **Scene Release** is a semi-structured serialization of metadata. It follows specific "Scene Rules" governed by release groups to ensure consistency.9 A typical release string encodes the title, year, resolution, source, audio codec, video codec, and release group, separated by delimiters. However, entropy exists. P2P networks and non-scene groups frequently violate these schemas, introducing obfuscation or "natural language" elements into the filename (e.g., "My Big Fat Greek Wedding 3 (2023) \[1080p\]\[Clean Audio\]").

The bridge between these two worlds requires a translation layer that understands that "2" and "Part II" are semantically identical in the context of sequels, yet distinct from "Season 2".11 This nuance is where traditional regex struggles and where Large Language Models (LLMs) and Vector Space Models excel.

## ---

**2\. Deep Analysis of Scene Release Taxonomy and Deterministic Parsing**

Before engineering the cognitive layers of the system, we must exhaustively map the input domain. Scene releases are not random; they follow a taxonomy that allows for a high degree of deterministic parsing. This "Tier 1" layer serves as the foundation of the architecture, handling the vast majority of standard inputs with negligible computational cost.

### **2.1 The Standard Scene Naming Schema**

A typical scene release roughly follows a positional schema, though variations are common. The effective parsing of these strings relies on identifying "Anchor Points"—tokens that have a high probability of fixed formatting, such as the Year or the SxxExx pattern.

| Component | Standard Pattern | Regex Pattern | Ambiguity Risk |
| :---- | :---- | :---- | :---- |
| **Title** | The.Matrix | ^.\*?(?=\\d{4}) | High. Can contain numbers/years. |
| **Year** | 1999 | (19|20)\\d{2} | Moderate. Can be confused with title numbers (e.g., "2012"). |
| **Resolution** | 1080p | \\d{3,4}\[pi\] | Low. |
| **Source** | BluRay | (BluRay|WEB-DL|HDTV) | Low. |
| **Codec** | x264 | \[hx\].?26 | Low. |
| **Group** | \-CiNEFiLE | \-\[A-Za-z0-9\]+$ | Moderate. Can define parsing mode (Anime vs. Standard). |

#### **2.1.1 The Title Block and The Anchor Strategy**

The title block is the most volatile component. It uses delimiters (periods, spaces, underscores) to separate words. A robust parser should not attempt to match the title directly from left to right, as the length is variable. Instead, the **Anchor Strategy** should be employed.

The parser scans the string for the **Year** (19|20)\\d{2} or the **Season/Episode** marker S\\d{2}E\\d{2}. These tokens act as pivots.

1. **Scan right-to-left** for the Anchor.  
2. Everything to the **left** of the anchor is the Candidate Title.  
3. Everything to the **right** of the anchor is Metadata.

This strategy isolates the volatile title string from the structured metadata. However, "Title Bleed" is a common failure mode where metadata terms appear in the title. For instance, a movie titled *Resolution* (2012) creates a conflict if the parser blindly scans for the word "Resolution".12 Similarly, the movie *2012* introduces ambiguity in year extraction.

To mitigate this, the deterministic parser in Go should utilize a **Token-Based Approach** rather than a single monolithic regex. The string is split into tokens, and each token is classified based on dictionary lookups (e.g., "x264" \= Codec). Tokens that fail classification and precede the anchor are aggregated into the title.

### **2.2 Go Implementation Strategies for Deterministic Parsing**

The Go standard library's regexp package uses the RE2 engine, which guarantees linear time execution $O(n)$ with respect to the input size.12 This is crucial for a downloading app that may process tens of thousands of RSS items per hour. Unlike PCRE (used in PHP/Python), RE2 does not support lookarounds or backreferences, which limits the complexity of single-pass regexes but prevents ReDoS (Regular Expression Denial of Service) attacks from malicious filenames.

A high-performance Go parser should implement a **Pipeline of Micro-Parsers**:

1. **Normalization:** Convert delimiters to spaces, collapse multiple spaces.  
2. **Anchor Detection:** Locate the Year or SxxExx.  
3. **Right-Side Extraction:** Iterate tokens after the anchor to extract Quality, Source, Audio, and Group using map-based lookups (Hash Maps) for $O(1)$ complexity, rather than iterating through a list of regexes.  
4. **Left-Side Extraction:** Treat the remainder as the title.

Existing Go libraries such as go-parse-torrent-name 13 and torrentnameparser 14 follow a "destructive" strategy, removing matched tokens from the string. While effective for simple cases, this destroys context. A non-destructive, positional approach is recommended for the "Robust" solution requested.

### **2.3 The "Propers" and Versioning**

Scene rules dictate that if a release is flawed, a new release must be tagged PROPER or REPACK. Some groups use REAL.PROPER to supersede a PROPER. A robust system must extract these tags and assign a numerical **Score** or **Rank** to the release.9

In Go, this can be modeled as a priority queue:

Go

const (  
    VersionStandard \= 0  
    VersionRepack   \= 1  
    VersionProper   \= 2  
    VersionReal     \= 3  
)

When reconciling multiple releases of the same content, the system sorts by (Quality, VersionRank, Date).

## ---

**3\. The Anime Anomaly: Absolute Numbering and Heuristic Detection**

A specific and significant challenge cited in the research is the handling of Anime releases, which often defy the standard SxxExx format in favor of **Absolute Numbering** (e.g., Episode 1035).5

### **3.1 The Absolute vs. Seasonal Conflict**

Standard parsers (like those in early versions of Sonarr) assume that any number following the title is a season or episode index. In the case of *One Piece \- 1035*, a naive parser might interpret this as Season 10, Episode 35, or simply fail to match.15 The \*arr stack addresses this by maintaining separate "Series Types" (Standard, Daily, Anime), but this requires user intervention.

A truly robust app should auto-detect the series type. This can be achieved through **Heuristic Analysis** of the release string features unique to the anime scene.

### **3.2 Heuristic Detection Algorithms**

The following features are strong indicators of an anime release:

1. **Group Headers:** Anime releases almost universally begin with the release group name enclosed in square brackets, e.g., \`\` or \[Erai-raws\].12 Standard scene releases place the group at the end.  
2. **CRC32 Hashes:** Anime releases typically include an 8-character hexadecimal checksum at the end of the filename, e.g., \`\` or (8F3E2A1D).  
3. **Absolute Numbering Pattern:** The pattern Title \- \\d{2,4} (hyphen-separated number) is characteristic of anime, whereas Title.S\\d{2} is characteristic of western TV.

Go Implementation:  
The parser should run a pre-flight check using these heuristics.

Go

func DetectSeriesType(name string) SeriesType {  
    if matchesRegex(name, \`^\\\[.\*?\\\]\`) && matchesRegex(name, \`\[0-9A-F\]{8}\`) {  
        return SeriesTypeAnime  
    }  
    return SeriesTypeStandard  
}

If SeriesTypeAnime is detected, the parser switches to a logic path that accepts \\d+ as an absolute episode number.

### **3.3 The XEM Mapping Problem**

Even if the parser correctly extracts "Episode 1035", this data must be mapped to metadata sources like TheTVDB, which often enforce seasonal structures (e.g., *One Piece* Season 21, Episode 144).5 This requires an intermediate mapping layer known in the community as **XEM (The Cross-Reference Engine)**.

The proposed system should integrate a local caching of XEM data. When an absolute number is parsed, the system queries the XEM map:  
Map(SeriesID, Absolute: 1035\) \-\> (Season: 21, Episode: 144).  
This allows the system to search for "Episode 1035" on anime trackers (Nyaa) while organizing the file as "S21E144" in the library, satisfying the "Extraction" requirement of the user query.18

## ---

**4\. Fuzzy Logic and Information Retrieval: Beyond Exact Matching**

Once the deterministic layer has extracted a "Candidate Title" (e.g., "Back to the Future Part II"), the system must match this against the "Human Titles" stored in its database. Exact string equality (==) is insufficient due to typos, formatting variations, and transliteration differences. This necessitates **Fuzzy Logic** and **Information Retrieval (IR)** techniques.20

### **4.1 Bleve: Embedded Full-Text Search in Go**

For a Go-based application, **Bleve** is the premier embedded text indexing library. It avoids the operational overhead of running a separate service like Elasticsearch or Solr, adhering to the "self-contained" design goal.20 Bleve allows the creation of an inverted index on disk, mapping terms to document IDs.

#### **4.1.1 Indexing Strategy for Media Titles**

To enable robust matching, titles should be indexed with a specific analyzer pipeline:

1. **Normalization:** Convert to lowercase.  
2. **Stop Word Removal:** Remove common articles ("the", "and", "a") which add noise to relevance scoring.  
3. **Stemming:** Reduce words to their root form (e.g., "returning" \-\> "return").  
4. **N-Gram Tokenization:** Break titles into overlapping character sequences (e.g., "matrix" \-\> "mat", "atr", "tri", "rix"). This is critical for handling typos and partial matches.

Architecture Decision:  
Using an Edge N-Gram tokenizer is particularly effective for "starts-with" queries, allowing users to type "Back to" and immediately retrieve "Back to the Future".

### **4.2 Handling Numeric and Linguistic Variations**

A common failure case in media matching is the representation of numbers: 2 vs II vs Two vs Second.12  
Solution: Implement a Synonym Filter in the Bleve mapping.

* Map ii, two, second, 2 to a canonical token \_num\_2.  
* Map &, and, plus to a canonical token \_conj\_and.

This normalizes the search space, allowing "Part II" to match "Part 2" seamlessly without complex regex rules.

### **4.3 Advanced String Distance Algorithms**

For lightweight checks without a full index—or for re-ranking the top results from Bleve—Go developers should utilize advanced string distance algorithms provided by libraries like go-edlib.23

* **Levenshtein Distance:** Measures the number of edits (insertions, deletions, substitutions) required to change one string into another. Good for general typos.  
* **Jaro-Winkler Distance:** A variation of Jaro distance that gives higher scores to strings that match at the beginning. This is highly effective for media titles, where the franchise name (e.g., "Star Wars") usually appears at the start.  
* **Sorensen-Dice Coefficient:** Measures similarity based on bigrams (pairs of adjacent letters). This is often more robust than Levenshtein for multi-word titles as it is less sensitive to word order inversions.

**Implementation Pattern:**

1. **Retrieve:** Use Bleve to get the Top 10 candidate movies.  
2. **Re-rank:** Calculate the Jaro-Winkler distance between the parsed release title and the candidate human titles.  
3. **Select:** Choose the match with the highest score, provided it exceeds a safety threshold (e.g., 0.9).

## ---

**5\. Tier 3: Semantic Search with Vector Embeddings (The Context Layer)**

This layer introduces the first major AI component. Traditional fuzzy matching looks for *character* overlap. Semantic matching looks for *meaning* overlap. This is crucial for matching "Fast 5" to "Fast & Furious 5" where character overlap might be low, or recognizing that "Avengers 4" is semantically equivalent to "Avengers: Endgame".1

### **5.1 Vector Space Models**

In a Vector Space Model, text is transformed into a high-dimensional vector (an array of floating-point numbers, e.g., 384 dimensions). The distance between two vectors (usually measured by **Cosine Similarity**) represents their semantic similarity.25

For media titles, this allows the system to disambiguate based on context. "The Rock" (1996 movie) and "Dwayne Johnson" (Actor) are semantically related in a vector space trained on movie metadata, enabling smarter search capabilities that go beyond simple title matching.

### **5.2 Embedding Models for Go**

To implement this in a Go app, we need a way to generate embeddings locally. We cannot rely on external APIs (like OpenAI's text-embedding-ada-002) if we want the app to be robust and offline-capable.

**Recommended Model:** **all-MiniLM-L6-v2**.27

* **Size:** \~80MB (very small).  
* **Dimensions:** 384\.  
* **Performance:** Highly optimized for sentence similarity tasks.

Inference Engine: ONNX Runtime.29  
Using the github.com/yalue/onnxruntime\_go\_examples bindings, the application can load the .onnx version of MiniLM and perform inference on the CPU with sub-50ms latency. This library uses CGO to link against the shared ONNX Runtime library (written in C++), providing near-native speeds.

### **5.3 Embedded Vector Databases in Go**

Once embeddings are generated, they must be stored and searched efficiently. A standard SQL database is inefficient for Nearest Neighbor search.

**Option A: Chromem-go** 27

* **Type:** Pure Go embedded vector database.  
* **Algorithm:** HNSW (Hierarchical Navigable Small World) graph or brute-force (for small datasets).  
* **Pros:** No CGO required, easy to compile cross-platform, supports persistence.  
* **Cons:** Newer, less mature than C++ alternatives.

**Option B: SQLite-vec** 31

* **Type:** SQLite extension.  
* **Algorithm:** Brute-force (with plans for indexing).  
* **Pros:** Deep integration with the existing SQLite database likely used by the \*arr app.  
* **Cons:** Requires CGO and complex build flags to bundle the extension.

**Recommendation:** **Chromem-go** is the superior choice for a "spirit of \*arr" app aiming for portability. It allows the embedding of the vector search logic directly into the binary without managing external shared libraries or extensions.

### **5.4 The Hybrid Search Strategy**

Vector search is powerful but imprecise with specific identifiers (e.g., distinguishing "Iron Man 2" from "Iron Man 3"). Keyword search (Bleve) is precise but brittle. The "Robust" solution is **Hybrid Search**.33

Reciprocal Rank Fusion (RRF) 35:  
This algorithm combines the results from the vector search and the keyword search.

$$Score(d) \= \\sum\_{r \\in R} \\frac{1}{k \+ rank(r, d)}$$

where $rank(r, d)$ is the rank of document $d$ in result set $r$, and $k$ is a constant (usually 60).  
By implementing RRF in Go, the system ensures that a result must be relevant *both* semantically and lexically to appear at the top, significantly reducing false positives.

## ---

**6\. Tier 4: Cognitive Parsing with Small Language Models (SLMs)**

When Tiers 1-3 fail—typically due to obfuscated, novel, or extremely messy naming conventions—the system escalates to the "Cognitive Layer." This involves embedding a Generative Large Language Model (LLM) to perform reasoning and extraction.

### **6.1 The "Small" LLM Revolution**

We do not need a massive server-grade model (like GPT-4). For text extraction, recent **Small Language Models (SLMs)** in the 1B-3B parameter range are sufficient and can run on consumer hardware.36

**Selected Model: Phi-3 Mini (3.8B)**.38

* **Architecture:** Transformer-based, highly optimized for reasoning.  
* **Quantization:** Using 4-bit quantization (Q4\_K\_M), the model size is reduced to \~2.3GB.  
* **Performance:** Capable of running on a modern CPU with 8GB RAM, producing tokens at acceptable speeds (5-10 tokens/sec) for background tasks.

### **6.2 Embedding LLMs via go-llama.cpp**

To run Phi-3 within a Go application, we utilize **go-llama.cpp** 39, which provides Go bindings for the industry-standard llama.cpp inference engine.

* **Mechanism:** The library uses CGO to link the C++ inference code.  
* **GGUF Format:** The model must be in GGUF format, which supports memory mapping (mmap) for fast loading and low memory overhead.  
* **GPU Offloading:** go-llama.cpp supports offloading layers to the GPU (Metal on macOS, CUDA on Nvidia, Vulkan on AMD/Intel) 40, which drastically improves performance.

### **6.3 Constrained Decoding: The Key to Robustness**

A raw LLM outputs unstructured text, which is difficult to integrate into a software pipeline. We need the LLM to output structured data (JSON). llama.cpp supports **GBNF (GGML BNF) Grammars** 41, which enforce strict syntactic constraints on the model's output.

The GBNF Grammar Strategy:  
By passing a GBNF grammar file alongside the prompt, we force the LLM to generate only valid JSON that conforms to our ReleaseInfo struct. The sampling engine will reject any token that would violate the grammar.  
**Example GBNF:**

Code snippet

root   ::= object  
object ::= "{" ws "\\"title\\"" ":" string "," "\\"year\\"" ":" number "," "\\"resolution\\"" ":" string "}"  
string ::= "\\"" (\[^"\]\*) "\\""  
number ::= \[0-9\]+  
ws     ::= \[ \\t\\n\]\*

**Prompt Engineering:**

"Analyze the following release string and extract the metadata into the specified JSON format. Fix any obfuscation."  
Input: My.Big.Fat.Greek.Wedding.3.2023.PART1.REAL.PROPER.INTERNAL.1080p...

**Constrained Output:**

JSON

{"title": "My Big Fat Greek Wedding 3", "year": 2023, "resolution": "1080p"}

This guarantees 100% parseable output, eliminating the "hallucination" risk regarding format.43

### **6.4 The "Cognitive Cleaner" Pattern**

Using the LLM for every download is inefficient. The architectural pattern here is the **Cognitive Cleaner**.

1. **Fast Path:** Regex attempts to parse the string.  
2. **Failure Trigger:** If Regex fails (confidence \< 0.5) OR if the result yields 0 search matches.  
3. **LLM Intervention:** The string is sent to the SLM.  
4. **Feedback:** The SLM returns a "Cleaned Title" and specific metadata.  
5. **Re-entry:** The Cleaned Title is fed back into the Bleve/Vector search index.

This ensures that the expensive operation (LLM inference) is only paid for the difficult 5-10% of cases, maintaining high overall system throughput.

## ---

**7\. Architectural Synthesis: The "Waterfall" Pipeline**

To create the "robust and elegant" solution requested, we arrange these technologies into a cascading **Waterfall Pipeline**. This optimizes for the "Fast Path" (microseconds) while ensuring the "Correct Path" (seconds) is available for complex cases.

### **Phase 1: Ingest & Fast Filter (The Firehose)**

* **Input:** High-volume RSS feeds or IRC announce strings.  
* **Mechanism:** Pure Go Regex (RE2) & Dictionary Lookups.  
* **Action:**  
  * Normalize string.  
  * Identify Anchor (Year/Season).  
  * Extract Metadata (Quality, Source, Group).  
  * **Heuristic Check:** Detect Anime (Absolute Numbering).  
* **Metrics:** Latency \~50µs. Success Rate \~90%.  
* **Decision:** If Confidence \> 0.9, **Accept & Queue**. Else, pass to Phase 2\.

### **Phase 2: Fuzzy Verification & Re-ranking**

* **Input:** Candidate Title from Phase 1\.  
* **Mechanism:** Bleve Search \+ Jaro-Winkler Distance.  
* **Action:**  
  * Query local DB for candidate movies.  
  * Re-rank candidates using string distance.  
* **Metrics:** Latency \~2ms.  
* **Decision:** If top match score \> 0.95, **Link & Return**. Else, pass to Phase 3\.

### **Phase 3: Semantic Disambiguation**

* **Input:** Ambiguous Title (e.g., "The Office").  
* **Mechanism:** Vector Embedding (ONNX) \+ Chromem-go Search.  
* **Action:**  
  * Generate embedding for candidate title.  
  * Perform Hybrid Search (Vector \+ Keyword) using Reciprocal Rank Fusion.  
* **Metrics:** Latency \~50ms.  
* **Decision:** If Cosine Similarity \> 0.88, **Link & Return**. Else, pass to Phase 4\.

### **Phase 4: Cognitive Extraction (The Safety Net)**

* **Input:** Failed/Obfuscated String.  
* **Mechanism:** Phi-3 Mini (GGUF) via go-llama.cpp with GBNF Grammar.  
* **Action:**  
  * Contextualize the string ("This is a release name...").  
  * Extract structured JSON.  
  * Feed cleaned title back to Phase 2\.  
* **Metrics:** Latency \~500ms \- 2s.  
* **Decision:** Final Attempt. If this fails, flag as "Manual Import Needed."

## ---

**8\. Implementation Strategy in Go**

### **8.1 Concurrency Patterns**

Go's concurrency primitives are ideal for this pipeline.

* **Worker Pools:** Use a buffered channel to ingest RSS items. Spawn a pool of N workers (where N \= CPU cores) for Phase 1-3.  
* **Semaphore for LLM:** The LLM (Phase 4\) is memory and compute-intensive. It must be protected by a **Semaphore** (or a worker pool of size 1\) to prevent the system from spawning 20 concurrent LLM instances and crashing the OS.39

Go

// Example Semaphore for LLM  
var llmSemaphore \= make(chan struct{}, 1) // Allow 1 concurrent inference

func CognitiveParse(input string) Result {  
    llmSemaphore \<- struct{}{} // Acquire  
    defer func() { \<-llmSemaphore }() // Release  
    // Run Inference...  
}

### **8.2 CGO and Build Complexity**

While Go strives for static binaries, incorporating llama.cpp and onnxruntime introduces CGO dependencies.

* **Dockerization:** The most elegant distribution method is a Docker container. This allows you to bundle the necessary shared libraries (libllama.so, libonnxruntime.so) and the GGUF model files within the image, abstracting the complexity from the end-user.  
* **Tags:** Use Go build tags to allow compiling a "lite" version without the AI components for users on low-end hardware (e.g., Raspberry Pi Zero). go build \-tags no\_llm.

### **8.3 Data Datasets for Training/Benchmarking**

To validate the system, one cannot rely on synthetic data. Real-world scene datasets are required.

* **MagnetDB:** A longitudinal torrent discovery dataset.44  
* **OpenSubtitles:** A massive corpus of movie titles and release names.45  
* **MIT Scene Parsing Benchmark:** While for images, the methodology of separating training/validation sets applies here.47

By utilizing these datasets, developers can benchmark the accuracy of the Regex parser vs. the Hybrid approach, fine-tuning the confidence thresholds for the "Waterfall" transitions.

## ---

**9\. Conclusion**

The architecture presented here represents a paradigm shift for the \*arr ecosystem. By moving from a strictly deterministic model to a hybrid probabilistic model, the application gains the resilience of AI without sacrificing the performance of Go.

The integration of **Phi-3 Mini** via **go-llama.cpp** allows the system to "read" release names with human-level comprehension, while **Chromem-go** and **Bleve** provide the navigational tools to map those names to the correct metadata. This tiered approach respects the resource constraints of the host machine—using expensive cognitive cycles only when necessary—resulting in a solution that is robust, elegant, and future-proof against the evolving landscape of scene naming conventions.

### **Recommended Stack Summary**

| Component | Technology | Role |
| :---- | :---- | :---- |
| **Language** | Go (Golang) 1.23+ | Core Logic, Concurrency, API. |
| **Regex Engine** | regexp (RE2) | Fast Path Parsing (90% traffic). |
| **Fuzzy Search** | blevesearch/bleve | Keyword Indexing & Search. |
| **Vector DB** | philippgille/chromem-go | Semantic Search & Disambiguation. |
| **Embedding Inference** | yalue/onnxruntime\_go\_examples | Running MiniLM-L6-v2. |
| **LLM Inference** | go-skynet/go-llama.cpp | Running Phi-3 for Cognitive Extraction. |
| **Data Mapping** | XEM / TheTVDB | Canonical Metadata Source. |

#### **Works cited**

1. What are Vector Embeddings? | A Comprehensive Vector Embeddings Guide \- Elastic, accessed January 18, 2026, [https://www.elastic.co/what-is/vector-embedding](https://www.elastic.co/what-is/vector-embedding)  
2. Search relevance tuning: Balancing keyword and semantic search \- Elasticsearch Labs, accessed January 18, 2026, [https://www.elastic.co/search-labs/blog/search-relevance-tuning-in-semantic-search](https://www.elastic.co/search-labs/blog/search-relevance-tuning-in-semantic-search)  
3. Manual search Parsing? \- Help & Support \- sonarr :: forums, accessed January 18, 2026, [https://forums.sonarr.tv/t/manual-search-parsing/20784](https://forums.sonarr.tv/t/manual-search-parsing/20784)  
4. Use a better parser for local files \- Feature Requests \- sonarr :: forums, accessed January 18, 2026, [https://forums.sonarr.tv/t/use-a-better-parser-for-local-files/18055](https://forums.sonarr.tv/t/use-a-better-parser-for-local-files/18055)  
5. \[Anime Plugin\] Integrating Absolute numbering \- Plugins \- Emby Community, accessed January 18, 2026, [https://emby.media/community/index.php?/topic/13284-anime-plugin-integrating-absolute-numbering/](https://emby.media/community/index.php?/topic/13284-anime-plugin-integrating-absolute-numbering/)  
6. Absolute episode number parsing issue : r/sonarr \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/sonarr/comments/1ao88yo/absolute\_episode\_number\_parsing\_issue/](https://www.reddit.com/r/sonarr/comments/1ao88yo/absolute_episode_number_parsing_issue/)  
7. What are common failure modes in semantic search systems? \- Milvus, accessed January 18, 2026, [https://milvus.io/ai-quick-reference/what-are-common-failure-modes-in-semantic-search-systems](https://milvus.io/ai-quick-reference/what-are-common-failure-modes-in-semantic-search-systems)  
8. Ask like a human: Implementing semantic search on Stack Overflow, accessed January 18, 2026, [https://stackoverflow.blog/2023/07/31/ask-like-a-human-implementing-semantic-search-on-stack-overflow/](https://stackoverflow.blog/2023/07/31/ask-like-a-human-implementing-semantic-search-on-stack-overflow/)  
9. Scene filename parser \- Stash-Docs, accessed January 18, 2026, [https://docs.stashapp.cc/in-app-manual/tasks/scenefilenameparser/](https://docs.stashapp.cc/in-app-manual/tasks/scenefilenameparser/)  
10. pr0pz/scene-release-parser: A library for parsing scene release names into human readable data. \- GitHub, accessed January 18, 2026, [https://github.com/pr0pz/scene-release-parser](https://github.com/pr0pz/scene-release-parser)  
11. Prowlarr search interactive search inaccurate \- Help & Support \- sonarr :: forums, accessed January 18, 2026, [https://forums.sonarr.tv/t/prowlarr-search-interactive-search-inaccurate/33692](https://forums.sonarr.tv/t/prowlarr-search-interactive-search-inaccurate/33692)  
12. How to modify radarr default parsing logic? \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/radarr/comments/1g3es6a/how\_to\_modify\_radarr\_default\_parsing\_logic/](https://www.reddit.com/r/radarr/comments/1g3es6a/how_to_modify_radarr_default_parsing_logic/)  
13. middelink/go-parse-torrent-name: Extract media information from a torrent filename \- GitHub, accessed January 18, 2026, [https://github.com/middelink/go-parse-torrent-name](https://github.com/middelink/go-parse-torrent-name)  
14. github.com/profchaos/torrentnameparser v0.5.1 on Go \- Libraries.io \- security & maintenance data for open source software, accessed January 18, 2026, [https://libraries.io/go/github.com%2Fprofchaos%2Ftorrentnameparser](https://libraries.io/go/github.com%2Fprofchaos%2Ftorrentnameparser)  
15. Anime series being added to sonarr as anime instead of standard. : r/Overseerr \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/Overseerr/comments/18yk8b4/anime\_series\_being\_added\_to\_sonarr\_as\_anime/](https://www.reddit.com/r/Overseerr/comments/18yk8b4/anime_series_being_added_to_sonarr_as_anime/)  
16. Anime search result not parsed with absolute number \- Help & Support \- sonarr :: forums, accessed January 18, 2026, [https://forums.sonarr.tv/t/anime-search-result-not-parsed-with-absolute-number/11377](https://forums.sonarr.tv/t/anime-search-result-not-parsed-with-absolute-number/11377)  
17. \[Anime\] How do I process files with absolute numbers on the command-line? \- FileBot, accessed January 18, 2026, [https://www.filebot.net/forums/viewtopic.php?t=13405](https://www.filebot.net/forums/viewtopic.php?t=13405)  
18. Don't reorder anime episodes when absolute numbers change on TVDB \#6547 \- GitHub, accessed January 18, 2026, [https://github.com/Sonarr/Sonarr/issues/6547](https://github.com/Sonarr/Sonarr/issues/6547)  
19. Prefer Standard over Absolute Numbering · Issue \#7246 \- GitHub, accessed January 18, 2026, [https://github.com/Sonarr/Sonarr/issues/7246](https://github.com/Sonarr/Sonarr/issues/7246)  
20. Build a fast search engine in Golang \- Kevin Coder, accessed January 18, 2026, [https://kevincoder.co.za/bleve-how-to-build-a-rocket-fast-search-engine](https://kevincoder.co.za/bleve-how-to-build-a-rocket-fast-search-engine)  
21. Fuzzy string matching in golang \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/golang/comments/1lo3fsc/fuzzy\_string\_matching\_in\_golang/](https://www.reddit.com/r/golang/comments/1lo3fsc/fuzzy_string_matching_in_golang/)  
22. Building a search engine in Golang: A comprehensive guide \- Meilisearch, accessed January 18, 2026, [https://www.meilisearch.com/blog/golang-search-engine](https://www.meilisearch.com/blog/golang-search-engine)  
23. Go-edlib : Edit distance and string comparison library, accessed January 18, 2026, [https://pkg.go.dev/github.com/hbollon/go-edlib](https://pkg.go.dev/github.com/hbollon/go-edlib)  
24. Vector indexes \- Cypher Manual \- Neo4j, accessed January 18, 2026, [https://neo4j.com/docs/cypher-manual/current/indexes/semantic-indexes/vector-indexes/](https://neo4j.com/docs/cypher-manual/current/indexes/semantic-indexes/vector-indexes/)  
25. Finding Similar Sentences Using Cosine Similarity | by Lince Mathew \- Medium, accessed January 18, 2026, [https://medium.com/@linz07m/finding-similar-sentences-using-cosine-similarity-cc343d142e66](https://medium.com/@linz07m/finding-similar-sentences-using-cosine-similarity-cc343d142e66)  
26. cosine\_similarity package \- github.com/ugurkorkmaz/multiversal/cosine\_similarity \- Go Packages, accessed January 18, 2026, [https://pkg.go.dev/github.com/ugurkorkmaz/multiversal/cosine\_similarity](https://pkg.go.dev/github.com/ugurkorkmaz/multiversal/cosine_similarity)  
27. philippgille/chromem-go: Embeddable vector database for Go with Chroma-like interface and zero third-party dependencies. In-memory with optional persistence. \- GitHub, accessed January 18, 2026, [https://github.com/philippgille/chromem-go](https://github.com/philippgille/chromem-go)  
28. Show HN: Chromem-go – Embeddable vector database for Go \- Hacker News, accessed January 18, 2026, [https://news.ycombinator.com/item?id=39941144](https://news.ycombinator.com/item?id=39941144)  
29. Qitmeer/llama.go: Go bindings to llama.cpp \- GitHub, accessed January 18, 2026, [https://github.com/Qitmeer/llama.go](https://github.com/Qitmeer/llama.go)  
30. yalue/onnxruntime\_go\_examples: Example applications using the onnxruntime\_go library. \- GitHub, accessed January 18, 2026, [https://github.com/yalue/onnxruntime\_go\_examples](https://github.com/yalue/onnxruntime_go_examples)  
31. asg017/sqlite-vec: A vector search SQLite extension that runs anywhere\! \- GitHub, accessed January 18, 2026, [https://github.com/asg017/sqlite-vec](https://github.com/asg017/sqlite-vec)  
32. Introducing sqlite-vec v0.1.0: a vector search SQLite extension that runs everywhere | Alex Garcia's Blog, accessed January 18, 2026, [https://alexgarcia.xyz/blog/2024/sqlite-vec-stable-release/index.html](https://alexgarcia.xyz/blog/2024/sqlite-vec-stable-release/index.html)  
33. Hybrid Search \- Salesforce Help, accessed January 18, 2026, [https://help.salesforce.com/s/articleView?id=data.c360\_a\_hybridsearch\_index.htm\&language=en\_US\&type=5](https://help.salesforce.com/s/articleView?id=data.c360_a_hybridsearch_index.htm&language=en_US&type=5)  
34. Announcing Hybrid Search General Availability in Mosaic AI Vector Search | Databricks Blog, accessed January 18, 2026, [https://www.databricks.com/blog/announcing-hybrid-search-general-availability-mosaic-ai-vector-search](https://www.databricks.com/blog/announcing-hybrid-search-general-availability-mosaic-ai-vector-search)  
35. Elastic Search 8.9: Combine vector, keyword, and semantic retrieval with hybrid search, accessed January 18, 2026, [https://www.elastic.co/blog/whats-new-elastic-enterprise-search-8-9-0](https://www.elastic.co/blog/whats-new-elastic-enterprise-search-8-9-0)  
36. Introducing Phi-3: Redefining what's possible with SLMs | Microsoft Azure Blog, accessed January 18, 2026, [https://azure.microsoft.com/en-us/blog/introducing-phi-3-redefining-whats-possible-with-slms/](https://azure.microsoft.com/en-us/blog/introducing-phi-3-redefining-whats-possible-with-slms/)  
37. How good is Phi-3-mini for everyone? : r/LocalLLaMA \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/LocalLLaMA/comments/1cbt78y/how\_good\_is\_phi3mini\_for\_everyone/](https://www.reddit.com/r/LocalLLaMA/comments/1cbt78y/how_good_is_phi3mini_for_everyone/)  
38. Phi-3-Mini-4K-Instruct ONNX-Web \- Optimized AI Model for Browser Inference \- AIKosh, accessed January 18, 2026, [https://aikosh.indiaai.gov.in/home/models/details/phi\_3\_mini\_4k\_instruct\_onnx\_web\_optimized\_ai\_model\_for\_browser\_inference.html](https://aikosh.indiaai.gov.in/home/models/details/phi_3_mini_4k_instruct_onnx_web_optimized_ai_model_for_browser_inference.html)  
39. LLMs for Go Developers: A Plug-and-Play Approach with llama.cpp | by Vadim Filin, accessed January 18, 2026, [https://medium.com/@filinvadim/llms-for-go-developers-a-plug-and-play-approach-with-llama-cpp-4ccccb6d04df](https://medium.com/@filinvadim/llms-for-go-developers-a-plug-and-play-approach-with-llama-cpp-4ccccb6d04df)  
40. ggml-org/llama.cpp: LLM inference in C/C++ \- GitHub, accessed January 18, 2026, [https://github.com/ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp)  
41. guidance-ai/llguidance: Super-fast Structured Outputs \- GitHub, accessed January 18, 2026, [https://github.com/guidance-ai/llguidance](https://github.com/guidance-ai/llguidance)  
42. llama.cpp/grammars/README.md at master · ggml-org/llama.cpp · GitHub, accessed January 18, 2026, [https://github.com/ggml-org/llama.cpp/blob/master/grammars/README.md](https://github.com/ggml-org/llama.cpp/blob/master/grammars/README.md)  
43. Grammar for structured output in llama.cpp: useful? : r/LocalLLaMA \- Reddit, accessed January 18, 2026, [https://www.reddit.com/r/LocalLLaMA/comments/1orjv37/grammar\_for\_structured\_output\_in\_llamacpp\_useful/](https://www.reddit.com/r/LocalLLaMA/comments/1orjv37/grammar_for_structured_output_in_llamacpp_useful/)  
44. MagnetDB: A Longitudinal Torrent Discovery Dataset with IMDb-Matched Movies and TV Shows \- arXiv, accessed January 18, 2026, [https://arxiv.org/html/2501.09275v1](https://arxiv.org/html/2501.09275v1)  
45. conversational-datasets/opensubtitles/README.md at master \- GitHub, accessed January 18, 2026, [https://github.com/PolyAI-LDN/conversational-datasets/blob/master/opensubtitles/README.md](https://github.com/PolyAI-LDN/conversational-datasets/blob/master/opensubtitles/README.md)  
46. OpenSubtitles Corpus \- OPUS \- Corpora, accessed January 18, 2026, [https://opus.nlpl.eu/OpenSubtitles/corpus/version/OpenSubtitles](https://opus.nlpl.eu/OpenSubtitles/corpus/version/OpenSubtitles)  
47. MIT Scene Parsing (train & val) \- Kaggle, accessed January 18, 2026, [https://www.kaggle.com/datasets/aprilsan/mit-scene-parsing-train-and-val](https://www.kaggle.com/datasets/aprilsan/mit-scene-parsing-train-and-val)  
48. CSAILVision/sceneparsing: Development kit for MIT Scene Parsing Benchmark \- GitHub, accessed January 18, 2026, [https://github.com/CSAILVision/sceneparsing](https://github.com/CSAILVision/sceneparsing)
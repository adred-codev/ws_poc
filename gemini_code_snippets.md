```python
def file_name(self):
        return self.file + "-" + str(self.suffix_index) + ".log"
```
```python
def gemini_llm():
  return Gemini(model="gemini-1.5-flash")
```
```python
class GeminiModel:
    """Class for the Gemini model."""

    def __init__(
        self,
        model_name: str = "gemini-2.0-flash-001",
        finetuned_model: bool = False,
        distribute_requests: bool = False,
        cache_name: str | None = None,
        temperature: float = 0.01,
        **kwargs,
    ):
        self.model_name = model_name
        self.finetuned_model = finetuned_model
        self.arguments = kwargs
        self.distribute_requests = distribute_requests
        self.temperature = temperature
        model_name = self.model_name
        if not self.finetuned_model and self.distribute_requests:
            random_region = random.choice(GEMINI_AVAILABLE_REGIONS)
            model_name = GEMINI_URL.format(
                GCP_PROJECT=GCP_PROJECT,
                region=random_region,
                model_name=self.model_name,
            )
        if cache_name is not None:
            cached_content = caching.CachedContent(cached_content_name=cache_name)
            self.model = GenerativeModel.from_cached_content(
                cached_content=cached_content
            )
        else:
            self.model = GenerativeModel(model_name=model_name)

    @retry(max_attempts=12, base_delay=2, backoff_factor=2)
    def call(self, prompt: str, parser_func=None) -> str:
        """Calls the Gemini model with the given prompt.

        Args:
            prompt (str): The prompt to call the model with.
            parser_func (callable, optional): A function that processes the LLM
              output. It takes the model"s response as input and returns the
              processed result.

        Returns:
            str: The processed response from the model.
        """
        response = self.model.generate_content(
            prompt,
            generation_config=GenerationConfig(
                temperature=self.temperature,
                **self.arguments,
            ),
            safety_settings=SAFETY_FILTER_CONFIG,
        ).text
        if parser_func:
            return parser_func(response)
        return response

    def call_parallel(
        self,
        prompts: List[str],
        parser_func: Optional[Callable[[str], str]] = None,
        timeout: int = 60,
        max_retries: int = 5,
    ) -> List[Optional[str]]:
        """Calls the Gemini model for multiple prompts in parallel using threads with retry logic.

        Args:
            prompts (List[str]): A list of prompts to call the model with.
            parser_func (callable, optional): A function to process each response.
            timeout (int): The maximum time (in seconds) to wait for each thread.
            max_retries (int): The maximum number of retries for timed-out threads.

        Returns:
            List[Optional[str]]:
            A list of responses, or None for threads that failed.
        """
        results = [None] * len(prompts)

        def worker(index: int, prompt: str):
            """Thread worker function to call the model and store the result with retries."""
            retries = 0
            while retries <= max_retries:
                try:
                    return self.call(prompt, parser_func)
                except Exception as e:  # pylint: disable=broad-exception-caught
                    print(f"Error for prompt {index}: {str(e)}")
                    retries += 1
                    if retries <= max_retries:
                        print(f"Retrying ({retries}/{max_retries}) for prompt {index}")
                        time.sleep(1)  # Small delay before retrying
                    else:
                        return f"Error after retries: {str(e)}"

        # Create and start one thread for each prompt
        with ThreadPoolExecutor(max_workers=len(prompts)) as executor:
            future_to_index = {
                executor.submit(worker, i, prompt): i
                for i, prompt in enumerate(prompts)
            }

            for future in as_completed(future_to_index, timeout=timeout):
                index = future_to_index[future]
                try:
                    results[index] = future.result()
                except Exception as e:  # pylint: disable=broad-exception-caught
                    print(f"Unhandled error for prompt {index}: {e}")
                    results[index] = "Unhandled Error"

        # Handle remaining unfinished tasks after the timeout
        for future in future_to_index:
            index = future_to_index[future]
            if not future.done():
                print(f"Timeout occurred for prompt {index}")
                results[index] = "Timeout"

        return results
```
```python
def setup_gemini_config():
    """
    Create a custom evaluation configuration using Gemini 2.0 Flash via OpenRouter
    """
    # Configure to use Gemini 2.0 Flash via OpenRouter
    evaluation_config = {
        "model_name": "google/gemini-2.0-flash-001",  # OpenRouter format for Gemini
        "provider": "openai_endpoint",  # Use OpenRouter as endpoint
        "openai_endpoint_url": "https://openrouter.ai/api/v1",
        "temperature": 0,  # Zero temp for consistent evaluation
    }

    print(f"Using Gemini 2.0 Flash for evaluation: {evaluation_config}")
    return evaluation_config
```
```python
def mock_gemini_session():
  """Mock Gemini session for testing."""
  return mock.AsyncMock()
```
```python
class Gemini(BaseLlm):
  """Integration for Gemini models.

  Attributes:
    model: The name of the Gemini model.
  """

  model: str = 'gemini-1.5-flash'

  @staticmethod
  @override
  def supported_models() -> list[str]:
    """Provides the list of supported models.

    Returns:
      A list of supported models.
    """

    return [
        r'gemini-.*',
        # fine-tuned vertex endpoint pattern
        r'projects\/.+\/locations\/.+\/endpoints\/.+',
        # vertex gemini long name
        r'projects\/.+\/locations\/.+\/publishers\/google\/models\/gemini.+',
    ]

  async def generate_content_async(
      self, llm_request: LlmRequest, stream: bool = False
  ) -> AsyncGenerator[LlmResponse, None]:
    """Sends a request to the Gemini model.

    Args:
      llm_request: LlmRequest, the request to send to the Gemini model.
      stream: bool = False, whether to do streaming call.

    Yields:
      LlmResponse: The model response.
    """
    self._preprocess_request(llm_request)
    self._maybe_append_user_content(llm_request)
    logger.info(
        'Sending out request, model: %s, backend: %s, stream: %s',
        llm_request.model,
        self._api_backend,
        stream,
    )
    logger.info(_build_request_log(llm_request))

    # add tracking headers to custom headers given it will override the headers
    # set in the api client constructor
    if llm_request.config and llm_request.config.http_options:
      if not llm_request.config.http_options.headers:
        llm_request.config.http_options.headers = {}
      llm_request.config.http_options.headers.update(self._tracking_headers)

    if stream:
      responses = await self.api_client.aio.models.generate_content_stream(
          model=llm_request.model,
          contents=llm_request.contents,
          config=llm_request.config,
      )
      response = None
      thought_text = ''
      text = ''
      usage_metadata = None
      # for sse, similar as bidi (see receive method in gemini_llm_connecton.py),
      # we need to mark those text content as partial and after all partial
      # contents are sent, we send an accumulated event which contains all the
      # previous partial content. The only difference is bidi rely on
      # complete_turn flag to detect end while sse depends on finish_reason.
      async for response in responses:
        logger.info(_build_response_log(response))
        llm_response = LlmResponse.create(response)
        usage_metadata = llm_response.usage_metadata
        if (
            llm_response.content
            and llm_response.content.parts
            and llm_response.content.parts[0].text
        ):
          part0 = llm_response.content.parts[0]
          if part0.thought:
            thought_text += part0.text
          else:
            text += part0.text
          llm_response.partial = True
        elif (thought_text or text) and (
            not llm_response.content
            or not llm_response.content.parts
            # don't yield the merged text event when receiving audio data
            or not llm_response.content.parts[0].inline_data
        ):
          parts = []
          if thought_text:
            parts.append(types.Part(text=thought_text, thought=True))
          if text:
            parts.append(types.Part.from_text(text=text))
          yield LlmResponse(
              content=types.ModelContent(parts=parts),
              usage_metadata=llm_response.usage_metadata,
          )
          thought_text = ''
          text = ''
        yield llm_response
      if (
          (text or thought_text)
          and response
          and response.candidates
          and response.candidates[0].finish_reason == types.FinishReason.STOP
      ):
        parts = []
        if thought_text:
          parts.append(types.Part(text=thought_text, thought=True))
        if text:
          parts.append(types.Part.from_text(text=text))
        yield LlmResponse(
            content=types.ModelContent(parts=parts),
            usage_metadata=usage_metadata,
        )

    else:
      response = await self.api_client.aio.models.generate_content(
          model=llm_request.model,
          contents=llm_request.contents,
          config=llm_request.config,
      )
      logger.info(_build_response_log(response))
      yield LlmResponse.create(response)

  @cached_property
  def api_client(self) -> Client:
    """Provides the api client.

    Returns:
      The api client.
    """
    return Client(
        http_options=types.HttpOptions(headers=self._tracking_headers)
    )

  @cached_property
  def _api_backend(self) -> GoogleLLMVariant:
    return (
        GoogleLLMVariant.VERTEX_AI
        if self.api_client.vertexai
        else GoogleLLMVariant.GEMINI_API
    )

  @cached_property
  def _tracking_headers(self) -> dict[str, str]:
    framework_label = f'google-adk/{version.__version__}'
    if os.environ.get(_AGENT_ENGINE_TELEMETRY_ENV_VARIABLE_NAME):
      framework_label = f'{framework_label}+{_AGENT_ENGINE_TELEMETRY_TAG}'
    language_label = 'gl-python/' + sys.version.split()[0]
    version_header_value = f'{framework_label} {language_label}'
    tracking_headers = {
        'x-goog-api-client': version_header_value,
        'user-agent': version_header_value,
    }
    return tracking_headers

  @cached_property
  def _live_api_version(self) -> str:
    if self._api_backend == GoogleLLMVariant.VERTEX_AI:
      # use beta version for vertex api
      return 'v1beta1'
    else:
      # use v1alpha for using API KEY from Google AI Studio
      return 'v1alpha'

  @cached_property
  def _live_api_client(self) -> Client:
    return Client(
        http_options=types.HttpOptions(
            headers=self._tracking_headers, api_version=self._live_api_version
        )
    )

  @contextlib.asynccontextmanager
  async def connect(self, llm_request: LlmRequest) -> BaseLlmConnection:
    """Connects to the Gemini model and returns an llm connection.

    Args:
      llm_request: LlmRequest, the request to send to the Gemini model.

    Yields:
      BaseLlmConnection, the connection to the Gemini model.
    """
    # add tracking headers to custom headers and set api_version given
    # the customized http options will override the one set in the api client
    # constructor
    if (
        llm_request.live_connect_config
        and llm_request.live_connect_config.http_options
    ):
      if not llm_request.live_connect_config.http_options.headers:
        llm_request.live_connect_config.http_options.headers = {}
      llm_request.live_connect_config.http_options.headers.update(
          self._tracking_headers
      )
      llm_request.live_connect_config.http_options.api_version = (
          self._live_api_version
      )

    llm_request.live_connect_config.system_instruction = types.Content(
        role='system',
        parts=[
            types.Part.from_text(text=llm_request.config.system_instruction)
        ],
    )
    llm_request.live_connect_config.tools = llm_request.config.tools
    async with self._live_api_client.aio.live.connect(
        model=llm_request.model, config=llm_request.live_connect_config
    ) as live_session:
      yield GeminiLlmConnection(live_session)

  def _preprocess_request(self, llm_request: LlmRequest) -> None:

    if self._api_backend == GoogleLLMVariant.GEMINI_API:
      # Using API key from Google AI Studio to call model doesn't support labels.
      if llm_request.config:
        llm_request.config.labels = None

      if llm_request.contents:
        for content in llm_request.contents:
          if not content.parts:
            continue
          for part in content.parts:
            _remove_display_name_if_present(part.inline_data)
            _remove_display_name_if_present(part.file_data)
```
```typescript
interface FileInfo {
  path: string;
  name: string;
  extension: string;
  type: FileType | "unknown";
  short_path?: string;
}
```
```python
def __init__(self, gemini_session: live.AsyncSession):
    self._gemini_session = gemini_session
```
```python
def __init__(self, file, name) -> None:
        self.file = file
        self.name = name
        self.suffix_index = 0
        while os.path.exists(self.file_name):
            self.suffix_index += 1
        self.stdout = None
        self.logfile = None
```
```jsx
<ReactMD
      className={className}
      components={{
        code: CodeBlock
      }}
    >
      {markdownContent}
    </ReactMD>
```
```python
def _sanitize_schema_formats_for_gemini(
    schema: dict[str, Any],
) -> dict[str, Any]:
  """Filters the schema to only include fields that are supported by JSONSchema."""
  supported_fields: set[str] = set(_ExtendedJSONSchema.model_fields.keys())
  schema_field_names: set[str] = {"items"}  # 'additional_properties' to come
  list_schema_field_names: set[str] = {
      "any_of",  # 'one_of', 'all_of', 'not' to come
  }
  snake_case_schema = {}
  dict_schema_field_names: tuple[str] = ("properties",)  # 'defs' to come
  for field_name, field_value in schema.items():
    field_name = _to_snake_case(field_name)
    if field_name in schema_field_names:
      snake_case_schema[field_name] = _sanitize_schema_formats_for_gemini(
          field_value
      )
    elif field_name in list_schema_field_names:
      snake_case_schema[field_name] = [
          _sanitize_schema_formats_for_gemini(value) for value in field_value
      ]
    elif field_name in dict_schema_field_names:
      snake_case_schema[field_name] = {
          key: _sanitize_schema_formats_for_gemini(value)
          for key, value in field_value.items()
      }
    # special handle of format field
    elif field_name == "format" and field_value:
      current_type = schema.get("type")
      if (
          # only "int32" and "int64" are supported for integer or number type
          (current_type == "integer" or current_type == "number")
          and field_value in ("int32", "int64")
          or
          # only 'enum' and 'date-time' are supported for STRING type"
          (current_type == "string" and field_value in ("date-time", "enum"))
      ):
        snake_case_schema[field_name] = field_value
    elif field_name in supported_fields and field_value is not None:
      snake_case_schema[field_name] = field_value

  return _sanitize_schema_type(snake_case_schema)
```
```javascript
function renderMarkdown(markdown) {
        if (!markdown) return '';

        // This is a very basic markdown renderer for fallback purposes
        let html = markdown;

        // Convert headers
        html = html.replace(/^# (.*$)/gm, '<h1>$1</h1>');
        html = html.replace(/^## (.*$)/gm, '<h2>$1</h2>');
        html = html.replace(/^### (.*$)/gm, '<h3>$1</h3>');
        html = html.replace(/^#### (.*$)/gm, '<h4>$1</h4>');
        html = html.replace(/^##### (.*$)/gm, '<h5>$1</h5>');

        // Convert code blocks
        html = html.replace(/```([\s\S]*?)```/g, '<pre><code>$1</code></pre>');

        // Convert inline code
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Convert bold
        html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');

        // Convert italic
        html = html.replace(/\*(.*?)\*/g, '<em>$1</em>');

        // Convert links
        html = html.replace(/\[(.*?)\]\((.*?)\)/g, '<a href="$2">$1</a>');

        // Convert paragraphs - this is simplistic
        html = html.replace(/\n\s*\n/g, '</p><p>');
        html = '<p>' + html + '</p>';

        // Fix potentially broken paragraph tags
        html = html.replace(/<\/p><p><\/p><p>/g, '</p><p>');
        html = html.replace(/<\/p><p><(h[1-5])/g, '</p><$1');
        html = html.replace(/<\/(h[1-5])><p>/g, '</$1>');

        return html;
    }
```
```javascript
function renderMarkdown(markdown) {
    if (!markdown) {
        return '<div class="alert alert-warning">No content available</div>';
    }

    try {
        // Use marked library if available
        if (typeof marked !== 'undefined') {
            // Configure marked options
            marked.setOptions({
                breaks: true,
                gfm: true,
                headerIds: true,
                smartLists: true,
                smartypants: true,
                highlight: function(code, language) {
                    // Use Prism for syntax highlighting if available
                    if (typeof Prism !== 'undefined' && Prism.languages[language]) {
                        return Prism.highlight(code, Prism.languages[language], language);
                    }
                    return code;
                }
            });

            // Parse markdown and return HTML
            const html = marked.parse(markdown);

            // Process any special elements like image references
            const processedHtml = processSpecialMarkdown(html);

            return `<div class="markdown-content">${processedHtml}</div>`;
        } else {
            // Basic fallback if marked is not available
            console.warn('Marked library not available. Using basic formatting.');
            const basic = markdown
                .replace(/\n\n/g, '<br><br>')
                .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
                .replace(/\*(.*?)\*/g, '<em>$1</em>')
                .replace(/\[(.*?)\]\((.*?)\)/g, '<a href="$2" target="_blank">$1</a>');

            return `<div class="markdown-content">${basic}</div>`;
        }
    } catch (error) {
        console.error('Error rendering markdown:', error);
        return `<div class="alert alert-danger">Error rendering content: ${error.message}</div>`;
    }
}
```
```python
def setup_gemini_config(api_key=None):
    """
    Create a configuration for using Gemini via OpenRouter.

    Args:
        api_key: OpenRouter API key. If None, will try to get from environment.

    Returns:
        Dictionary with Gemini configuration.
    """
    # Get API key from argument or environment
    if not api_key:
        api_key = os.environ.get("OPENAI_ENDPOINT_API_KEY")
        if not api_key:
            api_key = os.environ.get("LDR_LLM__OPENAI_ENDPOINT_API_KEY")

    if not api_key:
        logger.error("No API key found. Please provide an OpenRouter API key.")
        return None

    return {
        "model_name": "google/gemini-2.0-flash-001",  # OpenRouter format for Gemini
        "provider": "openai_endpoint",  # Use OpenRouter as endpoint
        "openai_endpoint_url": "https://openrouter.ai/api/v1",
        "api_key": api_key,
    }
```
```python
def name(self):
        return "file_search"
```
```python
class File:
  """A structure that contains a file name and its content."""

  name: str
  """
  The name of the file with file extension (e.g., "file.csv").
  """

  content: str
  """
  The base64-encoded bytes of the file content.
  """

  mime_type: str = 'text/plain'
  """
  The mime type of the file (e.g., "image/png").
  """
```
```javascript
m =>
                            m.value === modelValue || m.id === modelValue
```
```javascript
get name(){return this._name}
```
```python
class MetadataManager:
    """Handles metadata operations for indexing."""

    async def create_and_save(
        self,
        params: Dict[str, Any],
        ctx: Optional[Any] = None,
    ) -> IndexMetadata:
        """Create and save metadata for the indexed repository.

        Args:
            params: Dictionary containing metadata parameters
            ctx: Context object for progress tracking (optional)

        Returns:
            Created IndexMetadata object
        """
        if ctx:
            await ctx.info('Finalizing index metadata...')
            await ctx.report_progress(90, 100)

        # Get index size
        index_size = 0
        for root, _, files in os.walk(params['index_path']):
            for file in files:
                index_size += os.path.getsize(os.path.join(root, file))

        # Use output_path as repository_name if provided
        final_repo_name = params['config'].output_path or params['repository_name']

        metadata = IndexMetadata(
            repository_name=final_repo_name,
            repository_path=params['config'].repository_path,
            index_path=params['index_path'],
            created_at=datetime.now(),
            last_accessed=None,
            file_count=len(set(params['chunk_to_file'].values())),
            chunk_count=len(params['chunks']),
            embedding_model=params['embedding_model'],
            file_types=params['extension_stats'],
            total_tokens=None,
            index_size_bytes=index_size,
            last_commit_id=params['last_commit_id'],
            repository_directory=params['repo_files_path'],
        )

        # Save metadata
        metadata_path = os.path.join(params['index_path'], 'metadata.json')
        metadata_json = metadata.model_dump_json(indent=2)
        with open(metadata_path, 'w') as f:
            f.write(metadata_json)

        return metadata
```
```python
class TestFormatMarkdown:
    """Tests for the Markdown formatting functions."""

    def test_format_markdown_case_summary(self, support_case_data):
        """Test formatting a case summary in Markdown."""
        formatted_case = format_case(support_case_data)
        markdown = format_markdown_case_summary(formatted_case)

        # Verify key elements are present in the Markdown
        assert f"**Case ID**: {support_case_data['caseId']}" in markdown
        assert f"**Subject**: {support_case_data['subject']}" in markdown
        assert "## Recent Communications" in markdown

        # Verify communication details
        first_comm = support_case_data["recentCommunications"]["communications"][0]
        assert first_comm["body"] in markdown
        assert first_comm["submittedBy"] in markdown

    def test_format_markdown_services(self, services_response_data):
        """Test formatting services in Markdown."""
        formatted_services = format_services(services_response_data["services"])
        markdown = format_markdown_services(formatted_services)

        # Verify key elements are present in the Markdown
        assert "# AWS Services" in markdown

        # Verify first service
        first_service = services_response_data["services"][0]
        assert f"## {first_service['name']}" in markdown
        assert f"`{first_service['code']}`" in markdown

        # Verify categories
        if first_service["categories"]:
            assert "### Categories" in markdown
            first_category = first_service["categories"][0]
            assert f"`{first_category['code']}`" in markdown

    def test_format_markdown_severity_levels(self, severity_levels_response_data):
        """Test formatting severity levels in Markdown."""
        formatted_levels = format_severity_levels(severity_levels_response_data["severityLevels"])
        markdown = format_markdown_severity_levels(formatted_levels)

        # Verify key elements are present in the Markdown
        assert "# AWS Support Severity Levels" in markdown

        # Verify severity levels
        for level in severity_levels_response_data["severityLevels"]:
            assert f"**{level['name']}**" in markdown
            assert f"`{level['code']}`" in markdown

    def test_format_json_response(self):
        """Test JSON response formatting."""
        test_data = {"key1": "value1", "key2": {"nested": "value2"}, "key3": [1, 2, 3]}

        formatted = format_json_response(test_data)
        assert isinstance(formatted, str)
        parsed = json.loads(formatted)
        assert parsed == test_data
```
```python
def parse_markdown_documentation(
    content: str,
    asset_name: str,
    url: str,
    correlation_id: str = '',
) -> Dict[str, Any]:
    """Parse markdown documentation content for a resource.

    Args:
        content: The markdown content
        asset_name: The asset name
        url: The source URL for this documentation
        correlation_id: Identifier for tracking this request in logs

    Returns:
        Dictionary with parsed documentation details
    """
    start_time = time.time()
    logger.debug(f"[{correlation_id}] Parsing markdown documentation for '{asset_name}'")

    try:
        # Find the title (typically the first heading)
        title_match = re.search(r'^#\s+(.*?)$', content, re.MULTILINE)
        if title_match:
            title = title_match.group(1).strip()
            logger.debug(f"[{correlation_id}] Found title: '{title}'")
        else:
            title = f'AWS {asset_name}'
            logger.debug(f"[{correlation_id}] No title found, using default: '{title}'")

        # Find the main resource description section (all content after resource title before next heading)
        description = ''
        resource_heading_pattern = re.compile(
            rf'# {re.escape(asset_name)}\s+\(Resource\)\s*(.*?)(?=\n##|\Z)', re.DOTALL
        )
        resource_match = resource_heading_pattern.search(content)

        if resource_match:
            # Extract the description text and clean it up
            description = resource_match.group(1).strip()
            logger.debug(
                f"[{correlation_id}] Found resource description section: '{description[:100]}...'"
            )
        else:
            # Fall back to the description found on the starting markdown table of each github markdown page
            desc_match = re.search(r'description:\s*|-\n(.*?)\n---', content, re.MULTILINE)
            if desc_match:
                description = desc_match.group(1).strip()
                logger.debug(
                    f"[{correlation_id}] Using fallback description: '{description[:100]}...'"
                )
            else:
                description = f'Documentation for AWSCC {asset_name}'
                logger.debug(f'[{correlation_id}] No description found, using default')

        # Find all example snippets
        example_snippets = []

        # First try to extract from the Example Usage section
        example_section_match = re.search(r'## Example Usage\n([\s\S]*?)(?=\n## |\Z)', content)

        if example_section_match:
            # logger.debug(f"example_section_match: {example_section_match.group()}")
            example_section = example_section_match.group(1).strip()
            logger.debug(
                f'[{correlation_id}] Found Example Usage section ({len(example_section)} chars)'
            )

            # Find all subheadings in the Example Usage section with a more robust pattern
            subheading_list = list(
                re.finditer(r'### (.*?)[
]+(.*?)(?=###|
\Z)', example_section, re.DOTALL)
            )
            logger.debug(
                f'[{correlation_id}] Found {len(subheading_list)} subheadings in Example Usage section'
            )
            subheading_found = False

            # Check if there are any subheadings
            for match in subheading_list:
                # logger.info(f"subheading match: {match.group()}")
                subheading_found = True
                title = match.group(1).strip()
                subcontent = match.group(2).strip()

                logger.debug(
                    f"[{correlation_id}] Found subheading '{title}' with {len(subcontent)} chars content"
                )

                # Find code blocks in this subsection - pattern to match terraform code blocks
                code_match = re.search(r'```(?:terraform|hcl)?\s*(.*?)```', subcontent, re.DOTALL)
                if code_match:
                    code_snippet = code_match.group(1).strip()
                    example_snippets.append({'title': title, 'code': code_snippet})
                    logger.debug(
                        f"[{correlation_id}] Added example snippet for '{title}' ({len(code_snippet)} chars)"
                    )

            # If no subheadings were found, look for direct code blocks under Example Usage
            if not subheading_found:
                logger.debug(
                    f'[{correlation_id}] No subheadings found, looking for direct code blocks'
                )
                # Improved pattern for code blocks
                code_blocks = re.finditer(
                    r'```(?:terraform|hcl)?\s*(.*?)```', example_section, re.DOTALL
                )
                code_found = False

                for code_match in code_blocks:
                    code_found = True
                    code_snippet = code_match.group(1).strip()
                    example_snippets.append({'title': 'Example Usage', 'code': code_snippet})
                    logger.debug(
                        f'[{correlation_id}] Added direct example snippet ({len(code_snippet)} chars)'
                    )

                if not code_found:
                    logger.debug(
                        f'[{correlation_id}] No code blocks found in Example Usage section'
                    )
        else:
            logger.debug(f'[{correlation_id}] No Example Usage section found')

        if example_snippets:
            logger.info(f'[{correlation_id}] Found {len(example_snippets)} example snippets')
        else:
            logger.debug(f'[{correlation_id}] No example snippets found')

        # Extract Schema section
        schema_arguments = []
        schema_section_match = re.search(r'## Schema\n([\s\S]*?)(?=\n## |\Z)', content)
        if schema_section_match:
            schema_section = schema_section_match.group(1).strip()
            logger.debug(f'[{correlation_id}] Found Schema section ({len(schema_section)} chars)')

            # DO NOT Look for schema arguments directly under the main Schema section
            # args_under_main_section_match = re.search(r'(.*?)(?=
###|
##|$)', schema_section, re.DOTALL)
            # if args_under_main_section_match:
            #     args_under_main_section = args_under_main_section_match.group(1).strip()
            #     logger.debug(
            #         f'[{correlation_id}] Found arguments directly under the Schema section ({len(args_under_main_section)} chars)'
            #     )

            #     # Find arguments in this subsection
            #     arg_matches = re.finditer(
            #         r'-\s+`([^`]+)`\s+(.*?)(?=\n-\s+`|$)',
            #         args_under_main_section,
            #         re.DOTALL,
            #     )
            #     arg_list = list(arg_matches)
            #     logger.debug(
            #         f'[{correlation_id}] Found {len(arg_list)} arguments directly under the Argument Reference section'
            #     )

            #     for match in arg_list:
            #         arg_name = match.group(1).strip()
            #         arg_desc = match.group(2).strip() if match.group(2) else None
            #         # Do not add arguments that do not have a description
            #         if arg_name is not None and arg_desc is not None:
            #             schema_arguments.append({'name': arg_name, 'description': arg_desc, 'schema_section': "main"})
            #         logger.debug(
            #             f"[{correlation_id}] Added argument '{arg_name}': '{arg_desc[:50]}...' (truncated)"
            #         )

            # Now, Find all subheadings in the Argument Reference section with a more robust pattern
            subheading_list = list(
                re.finditer(r'### (.*?)[
]+(.*?)(?=###|
\Z)', schema_section, re.DOTALL)
            )
            logger.debug(
                f'[{correlation_id}] Found {len(subheading_list)} subheadings in Argument Reference section'
            )
            subheading_found = False

            # Check if there are any subheadings
            for match in subheading_list:
                subheading_found = True
                title = match.group(1).strip()
                subcontent = match.group(2).strip()
                logger.debug(
                    f"[{correlation_id}] Found subheading '{title}' with {len(subcontent)} chars content"
                )

                # Find arguments in this subsection
                arg_matches = re.finditer(
                    r'-\s+`([^`]+)`\s+(.*?)(?=\n-\s+`|$)',
                    subcontent,
                    re.DOTALL,
                )
                arg_list = list(arg_matches)
                logger.debug(
                    f'[{correlation_id}] Found {len(arg_list)} arguments in subheading {title}'
                )

                for match in arg_list:
                    arg_name = match.group(1).strip()
                    arg_desc = match.group(2).strip() if match.group(2) else None
                    # Do not add arguments that do not have a description
                    if arg_name is not None and arg_desc is not None:
                        schema_arguments.append(
                            {'name': arg_name, 'description': arg_desc, 'argument_section': title}
                        )
                    else:
                        logger.debug(
                            f"[{correlation_id}] Added argument '{arg_name}': '{arg_desc[:50] if arg_desc else 'No description found'}...' (truncated)"
                        )

            schema_arguments = schema_arguments if schema_arguments else None
            if schema_arguments:
                logger.info(
                    f'[{correlation_id}] Found {len(schema_arguments)} arguments across all sections'
                )
        else:
            logger.debug(f'[{correlation_id}] No Schema section found')

        # Return the parsed information
        parse_time = time.time() - start_time
        logger.debug(f'[{correlation_id}] Markdown parsing completed in {parse_time:.2f} seconds')

        return {
            'title': title,
            'description': description,
            'example_snippets': example_snippets if example_snippets else None,
            'url': url,
            'schema_arguments': schema_arguments,
        }

    except Exception as e:
        logger.exception(f'[{correlation_id}] Error parsing markdown content')
        logger.error(f'[{correlation_id}] Error type: {type(e).__name__}, message: {str(e)}')

        # Return partial info if available
        return {
            'title': f'AWSCC {asset_name}',
            'description': f'Documentation for AWSCC {asset_name} (Error parsing details: {str(e)})',
            'url': url,
            'example_snippets': None,
            'schema_arguments': None,
        }
```
```python
def name(self):
        return "code_interpreter"
```
```python
def name(self):
        return "image_generation"
```
```markdown
[Title] : AWSLABS.CORE-MCP-SERVER - How to translate a user query into AWS expert advice

[Section] : 5. Tool Usage Strategy
1. Initial Analysis: 
This is the implementation in md
# Understanding the user's requirements
<use_mcp_tool>
<server_name>awslabs.core-mcp-server</server_name>
<tool_name>prompt_understanding</tool_name>
<arguments>
{}
</arguments>
</use_mcp_tool>

1. Domain Research: 
This is the implementation in md
# Getting domain guidance
<use_mcp_tool>
<server_name>awslabs.bedrock-kb-retrieval-mcp-server</server_name>
<tool_name>QueryKnowledgeBases</tool_name>
<arguments>
{
  "query": "what services are allowed internally on aws",
  "knowledge_base_id": "KBID",
  "number_of_results": 10
}
</arguments>
</use_mcp_tool>

1. Architecture Planning: 
This is the implementation in md
# Getting CDK infrastructure guidance
<use_mcp_tool>
<server_name>awslabs.cdk-mcp-server</server_name>
<tool_name>CDKGeneralGuidance</tool_name>
<arguments>
{}
</arguments>
</use_mcp_tool>


```
```markdown
[Title] : AWSLABS.CORE-MCP-SERVER - How to translate a user query into AWS expert advice

[Section] : 6. Additional MCP Server Tools Examples

[Subsection] : 6.1 Nova Canvas MCP Server
Generate images for UI or solution architecture diagrams:This is the implementation in md
# Generating architecture visualization
<use_mcp_tool>
<server_name>awslabs.nova-canvas-mcp-server</server_name>
<tool_name>generate_image</tool_name>
<arguments>
{
  "prompt": "3D isometric view of AWS cloud architecture with Lambda functions, API Gateway, and DynamoDB tables, professional technical diagram style",
  "negative_prompt": "text labels, blurry, distorted",
  "width": 1024,
  "height": 1024,
  "quality": "premium",
  "workspace_dir": "/path/to/workspace"
}
</arguments>
</use_mcp_tool>


[Subsection] : 6.2 AWS Cost Analysis MCP Server
Get pricing information for AWS services:This is the implementation in md
# Getting pricing information
<use_mcp_tool>
<server_name>awslabs.cost-analysis-mcp-server</server_name>
<tool_name>get_pricing_from_web</tool_name>
<arguments>
{
  "service_code": "lambda"
}
</arguments>
</use_mcp_tool>


[Subsection] : 6.3 AWS Documentation MCP Server
Search for AWS documentation:This is the implementation in md
# Searching AWS documentation
<use_mcp_tool>
<server_name>awslabs.aws-documentation-mcp-server</server_name>
<tool_name>search_documentation</tool_name>
<arguments>
{
  "search_phrase": "Lambda function URLs",
  "limit": 5
}
</arguments>
</use_mcp_tool>


[Subsection] : 6.4 Terraform MCP Server
Execute Terraform commands and search for infrastructure documentation:This is the implementation in md
# Execute Terraform commands
<use_mcp_tool>
<server_name>awslabs.terraform-mcp-server</server_name>
<tool_name>ExecuteTerraformCommand</tool_name>
<arguments>
{
  "command": "plan",
  "working_directory": "/path/to/terraform/project",
  "variables": {
    "environment": "dev",
    "region": "us-west-2"
  }
}
</arguments>
</use_mcp_tool>

This is the implementation in md
# Search AWSCC provider documentation
<use_mcp_tool>
<server_name>awslabs.terraform-mcp-server</server_name>
<tool_name>SearchAwsccProviderDocs</tool_name>
<arguments>
{
  "asset_name": "awscc_lambda_function",
  "asset_type": "resource"
}
</arguments>
</use_mcp_tool>

This is the implementation in md
# Search for user-provided Terraform modules
<use_mcp_tool>
<server_name>awslabs.terraform-mcp-server</server_name>
<tool_name>SearchUserProvidedModule</tool_name>
<arguments>
{
  "module_url": "terraform-aws-modules/vpc/aws",
  "version": "5.0.0"
}
</arguments>
</use_mcp_tool>

Example Workflow:1. Research industry basics using AWS documentation search 
2. Identify common patterns and requirements 
3. Search AWS docs for specific solutions 
4. Use read_documentation to deep dive into relevant documentation 
5. Map findings to AWS services and patterns 
Key Research Areas:1. Industry-specific compliance requirements 
2. Common technical challenges 
3. Established solution patterns 
4. Performance requirements 
5. Security considerations 
6. Cost sensitivity 
7. Integration requirements 
Remember: The goal is to translate general application requirements into specific, modern AWS services and patterns while considering scalability, security, and cost-effectiveness. if any MCP server referenced here is not avalaible, ask the user if they would like to install it
```
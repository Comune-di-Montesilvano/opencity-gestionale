package opencity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const pageSize = 100

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Login autentica su OpenCity e ritorna il JWT.
// Usato dal web server (handler login) e dal CLI batch.
func Login(baseURL, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Post(baseURL+"/lang/api/auth", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("credenziali non valide")
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("auth decode: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("auth: token vuoto")
	}
	return result.Token, nil
}

// NewClient crea un client autenticato con JWT già ottenuto via Login.
func NewClient(baseURL, jwt string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      jwt,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(path string, params url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.httpClient.Do(req)
}

func (c *Client) post(path string, body any) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

type PagedResponse struct {
	Meta struct {
		Count int `json:"count"`
	} `json:"meta"`
	Data []json.RawMessage `json:"data"`
}

// FetchAllApplications recupera tutte le istanze di un servizio con paginazione automatica.
func (c *Client) FetchAllApplications(serviceID string, onProgress func(fetched, total int)) ([]json.RawMessage, error) {
	var all []json.RawMessage
	offset := 0

	for {
		params := url.Values{
			"version":    {"2"},
			"service_id": {serviceID},
			"limit":      {fmt.Sprintf("%d", pageSize)},
			"offset":     {fmt.Sprintf("%d", offset)},
		}
		resp, err := c.get("/lang/api/applications", params)
		if err != nil {
			return nil, fmt.Errorf("fetch offset %d: %w", offset, err)
		}
		var page PagedResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode offset %d (http %d): %w", offset, resp.StatusCode, err)
		}
		resp.Body.Close()

		all = append(all, page.Data...)
		if onProgress != nil {
			onProgress(len(all), page.Meta.Count)
		}
		if len(all) >= page.Meta.Count || len(page.Data) == 0 {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// FetchSampleApplication recupera la prima applicazione di un servizio (per il viewer JSON nel wizard).
func (c *Client) FetchSampleApplication(serviceID string) (*Application, error) {
	params := url.Values{
		"version":    {"2"},
		"service_id": {serviceID},
		"limit":      {"1"},
		"offset":     {"0"},
	}
	resp, err := c.get("/lang/api/applications", params)
	if err != nil {
		return nil, fmt.Errorf("fetch sample: %w", err)
	}
	defer resp.Body.Close()
	var page PagedResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("fetch sample decode: %w", err)
	}
	if len(page.Data) == 0 {
		return nil, fmt.Errorf("nessuna istanza disponibile per questo servizio")
	}
	var app Application
	if err := json.Unmarshal(page.Data[0], &app); err != nil {
		return nil, fmt.Errorf("unmarshal sample: %w", err)
	}
	return &app, nil
}

// FetchApplicationAtOffset recupera l'istanza all'offset N e il conteggio totale.
// Usato dalla navigazione prev/next nel wizard step 3.
func (c *Client) FetchApplicationAtOffset(serviceID string, offset int) (*Application, int, error) {
	params := url.Values{
		"version":    {"2"},
		"service_id": {serviceID},
		"limit":      {"1"},
		"offset":     {fmt.Sprintf("%d", offset)},
	}
	resp, err := c.get("/lang/api/applications", params)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch at offset %d: %w", offset, err)
	}
	defer resp.Body.Close()
	var page PagedResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, 0, fmt.Errorf("decode at offset %d: %w", offset, err)
	}
	if len(page.Data) == 0 {
		return nil, page.Meta.Count, fmt.Errorf("nessuna istanza all'offset %d", offset)
	}
	var app Application
	if err := json.Unmarshal(page.Data[0], &app); err != nil {
		return nil, page.Meta.Count, fmt.Errorf("unmarshal at offset %d: %w", offset, err)
	}
	return &app, page.Meta.Count, nil
}

// FetchApplication recupera una singola istanza per ID.
func (c *Client) FetchApplication(id string) (*Application, error) {
	params := url.Values{"version": {"2"}}
	resp, err := c.get("/lang/api/applications/"+id, params)
	if err != nil {
		return nil, fmt.Errorf("fetch application %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("istanza %s non trovata", id)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch application %s: HTTP %d", id, resp.StatusCode)
	}
	var app Application
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, fmt.Errorf("unmarshal application %s: %w", id, err)
	}
	return &app, nil
}


// Approve accetta una pratica su OpenCity con il messaggio fornito.
func (c *Client) Approve(applicationID, message string) error {
	return c.transition(applicationID, "accept", message)
}

// Reject rifiuta una pratica su OpenCity con il messaggio fornito.
func (c *Client) Reject(applicationID, message string) error {
	return c.transition(applicationID, "reject", message)
}

func (c *Client) transition(applicationID, action, message string) error {
	path := fmt.Sprintf("/lang/api/applications/%s/transition/%s", applicationID, action)
	resp, err := c.post(path, map[string]string{"message": message})
	if err != nil {
		return fmt.Errorf("transition %s %s: %w", action, applicationID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("transition %s %s: HTTP %d", action, applicationID, resp.StatusCode)
	}
	return nil
}

// OperatorUser contiene i dati dell'operatore loggato (da GET /lang/api/users/{id}).
type OperatorUser struct {
	ID                string   `json:"id"`
	Username          string   `json:"username"`
	Email             string   `json:"email"`
	GivenName         string   `json:"given_name"`
	FamilyName        string   `json:"family_name"`
	Role              string   `json:"role"` // "operator" | "admin"
	EnabledServiceIDs []string `json:"enabled_services_ids"`
}

// GetUser recupera i dati dell'operatore per user_id (ottenuto dal JWT).
func (c *Client) GetUser(userID string) (*OperatorUser, error) {
	resp, err := c.get("/lang/api/users/"+userID, url.Values{"version": {"2"}})
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", userID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("get user: accesso negato")
	}
	var u OperatorUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("get user decode: %w", err)
	}
	return &u, nil
}

// FetchServices recupera la lista dei servizi disponibili (per il setup wizard).
func (c *Client) FetchServices() ([]json.RawMessage, error) {
	resp, err := c.get("/lang/api/services", url.Values{"version": {"2"}})
	if err != nil {
		return nil, fmt.Errorf("fetch services: %w", err)
	}
	defer resp.Body.Close()
	var result []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("fetch services decode: %w", err)
	}
	return result, nil
}

type Branding struct {
	Nome    string `json:"nome"`
	Logo    string `json:"logo"`
	Favicon string `json:"favicon"`
}

// FetchBranding recupera le informazioni sul branding/tenant da OpenCity.
func FetchBranding(baseURL string) (*Branding, error) {
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Get(baseURL + "/lang/api/tenants/info")
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data struct {
		Name string   `json:"name"`
		Meta []string `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	b := &Branding{
		Nome: data.Name,
	}

	if len(data.Meta) > 0 {
		var m struct {
			Favicon string `json:"favicon"`
			Logo    string `json:"logo"`
		}
		if err := json.Unmarshal([]byte(data.Meta[0]), &m); err == nil {
			b.Favicon = m.Favicon
			b.Logo = m.Logo
		}
	}

	return b, nil
}

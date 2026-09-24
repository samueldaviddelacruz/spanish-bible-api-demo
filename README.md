
# 📖 Bible API (API de la Biblia)

A simple RESTful API to access Bible verses and metadata.  
Una API RESTful sencilla para acceder a versículos y metadatos de la Biblia.

---

## 🌐 Overview / Descripción general

**English:**  
This project is a Bible API built with [Go](https://golang.org/) using the [Huma](https://github.com/danielgtaylor/huma) framework. It serves Bible data stored in a local SQLite database. It supports fetching verses, chapters, and metadata.

**Español:**  
Este proyecto es una API de la Biblia construida con [Go](https://golang.org/) usando el framework [Huma](https://github.com/danielgtaylor/huma). Sirve datos bíblicos almacenados en una base de datos SQLite. Soporta la consulta de versículos, capítulos y libros.

---

## 📖 Translations / Traducciones

**English:**  
Three Spanish translations are served, one SQLite database each. Every endpoint takes `?version=`,
and without it the API answers with **RVR1960**. `GET /api/versions` lists what is available. Book IDs
carry the translation too (`spa-LBLA:Gen.1.1`), so a URL says which text it wants.

**Español:**  
Se sirven tres traducciones al español, una base de datos SQLite para cada una. Todos los endpoints
aceptan `?version=`, y sin él se responde con **RVR1960**. `GET /api/versions` devuelve las
disponibles. Los identificadores de libro también llevan la traducción (`spa-LBLA:Gen.1.1`), así que
una URL dice qué texto quiere.

| | | |
| --- | --- | --- |
| **RVR1960** | Reina-Valera 1960 | `?version=RVR1960` (default / por defecto) |
| **LBLA** | La Biblia de las Américas | `?version=LBLA` |
| **NVI** | Nueva Versión Internacional | `?version=NVI` |

```bash
curl "http://localhost:8888/api/versions"
curl "http://localhost:8888/api/books/spa-NVI:Gen/verses/chapter/1/verse/3?version=NVI"
```

---

## 🏗️ Technologies / Tecnologías

- [Go](https://golang.org/)
- [Huma](https://github.com/danielgtaylor/huma)
- SQLite

---

## 🚀 Getting Started / Primeros pasos

### English

1. **Clone the repository:**

   ```bash
   git clone https://github.com/samueldaviddelacruz/spanish-bible-api-demo.git
   cd spanish-bible-api-demo
   ```

2. **Run the API:**

   ```bash
   go run main.go
   ```

3. The API should be available at: `http://localhost:8888`

---

### Español

1. **Clonar el repositorio:**

   ```bash
   git clone https://github.com/samueldaviddelacruz/spanish-bible-api-demo.git
   cd spanish-bible-api-demo
   ```

2. **Ejecutar la API:**

   ```bash
   go run main.go
   ```

3. La API estará disponible en: `http://localhost:8888`

---

## 📚 Documentation/documentación

- [Documentation](https://ajphchgh0i.execute-api.us-west-2.amazonaws.com/dev/docs) 

---

## 📄 License / Licencia

MIT License

---

## ✝️ Credits / Créditos

- Bible content from public domain or properly licensed sources.
- API developed using Huma and Go.

---

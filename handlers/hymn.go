package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"fresh-words-backend/db"
	"fresh-words-backend/models"
	"fresh-words-backend/services"
	"fresh-words-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func cleanKey(raw string) string {
	return strings.TrimSpace(regexp.MustCompile(`(?i)^(?:key\s*:\s*)+`).ReplaceAllString(raw, ""))
}

type CreateHymnRequest struct {
	Number   int         `json:"number" binding:"required"`
	Title    string      `json:"title" binding:"required"`
	Chorus   string      `json:"chorus"`
	Verses   interface{} `json:"verses" binding:"required"` // array of strings or JSON string
	Category string      `json:"category"`
	Author   string      `json:"author"`
	Key      string      `json:"key"`
}

type UpdateHymnRequest struct {
	Number   int         `json:"number"`
	Title    string      `json:"title"`
	Chorus   string      `json:"chorus"`
	Verses   interface{} `json:"verses"`
	Category string      `json:"category"`
	Author   string      `json:"author"`
	Key      string      `json:"key"`
}

// helper to serialize verses to JSON string
func normalizeVerses(val interface{}) (string, error) {
	if val == nil {
		return "[]", nil
	}
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			return trimmed, nil
		}
		// If newline-separated stanzas (separated by \n\n)
		parts := strings.Split(trimmed, "\n\n")
		var clean []string
		for _, p := range parts {
			if strings.TrimSpace(p) != "" {
				clean = append(clean, strings.TrimSpace(p))
			}
		}
		bytes, err := json.Marshal(clean)
		return string(bytes), err
	case []interface{}:
		var clean []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				clean = append(clean, strings.TrimSpace(s))
			}
		}
		bytes, err := json.Marshal(clean)
		return string(bytes), err
	case []string:
		bytes, err := json.Marshal(v)
		return string(bytes), err
	default:
		bytes, err := json.Marshal(v)
		return string(bytes), err
	}
}

// GetHymnsHandler handles paginated and searchable hymn list for clients & admin
func GetHymnsHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	category := strings.TrimSpace(c.Query("category"))
	q := strings.TrimSpace(c.Query("q"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 50
	}
	offset := (page - 1) * limit

	query := db.DB.Model(&models.Hymn{})

	if category != "" && category != "All" {
		query = query.Where("LOWER(category) = LOWER(?)", category)
	}

	if q != "" {
		if num, err := strconv.Atoi(q); err == nil {
			query = query.Where("number = ? OR LOWER(title) LIKE LOWER(?) OR LOWER(verses) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?)",
				num, "%"+q+"%", "%"+q+"%", "%"+q+"%")
		} else {
			likeQuery := "%" + q + "%"
			query = query.Where("LOWER(title) LIKE LOWER(?) OR LOWER(verses) LIKE LOWER(?) OR LOWER(chorus) LIKE LOWER(?) OR LOWER(author) LIKE LOWER(?)",
				likeQuery, likeQuery, likeQuery, likeQuery)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to count hymns", err.Error())
		return
	}

	var hymns []models.Hymn
	if err := query.Order("number asc").Limit(limit).Offset(offset).Find(&hymns).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to fetch hymns", err.Error())
		return
	}

	utils.SendPaginated(c, http.StatusOK, "Hymns retrieved successfully", hymns, total, page, limit)
}

// GetAllHymnsHandler returns all hymns for offline client caching
func GetAllHymnsHandler(c *gin.Context) {
	var hymns []models.Hymn
	if err := db.DB.Order("number asc").Find(&hymns).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to fetch all hymns", err.Error())
		return
	}

	// If no hymns in database, seed defaults and return
	if len(hymns) == 0 {
		SeedDefaultHymns()
		db.DB.Order("number asc").Find(&hymns)
	}

	utils.SendSuccess(c, http.StatusOK, "All hymns retrieved successfully", hymns)
}

// GetHymnByIDHandler retrieves a single hymn by UUID or Number
func GetHymnByIDHandler(c *gin.Context) {
	param := strings.TrimSpace(c.Param("id"))

	var hymn models.Hymn
	var err error

	if hymnNum, numErr := strconv.Atoi(param); numErr == nil {
		err = db.DB.Where("number = ?", hymnNum).First(&hymn).Error
	} else if u, uuidErr := uuid.Parse(param); uuidErr == nil {
		err = db.DB.Where("id = ?", u).First(&hymn).Error
	} else {
		utils.SendError(c, http.StatusBadRequest, "Invalid hymn identifier", nil)
		return
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Hymn not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Failed to fetch hymn", err.Error())
		return
	}

	hymnToReturn := hymn
	lang := c.Query("lang")
	if lang != "" && lang != "en" {
		if trans, err := services.TranslateHymn(&hymnToReturn, lang); err == nil && trans != nil {
			hymnToReturn = *trans
		}
	}

	utils.SendSuccess(c, http.StatusOK, "Hymn retrieved successfully", hymnToReturn)
}

// TranslateHymnHandler translates a hymn on-demand by ID or Number
func TranslateHymnHandler(c *gin.Context) {
	param := strings.TrimSpace(c.Param("id"))
	lang := c.DefaultQuery("lang", "fr")

	var hymn models.Hymn
	var err error

	if hymnNum, numErr := strconv.Atoi(param); numErr == nil {
		err = db.DB.Where("number = ?", hymnNum).First(&hymn).Error
	} else if u, uuidErr := uuid.Parse(param); uuidErr == nil {
		err = db.DB.Where("id = ?", u).First(&hymn).Error
	} else {
		utils.SendError(c, http.StatusBadRequest, "Invalid hymn identifier", nil)
		return
	}

	if err != nil {
		utils.SendError(c, http.StatusNotFound, "Hymn not found", nil)
		return
	}

	translated, err := services.TranslateHymn(&hymn, lang)
	if err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Translation failed", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Hymn translated successfully", translated)
}

// AdminCreateHymnHandler creates a new hymn
func AdminCreateHymnHandler(c *gin.Context) {
	var req CreateHymnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Validation error", err.Error())
		return
	}

	// Check if hymn number already exists
	var count int64
	db.DB.Model(&models.Hymn{}).Where("number = ?", req.Number).Count(&count)
	if count > 0 {
		utils.SendError(c, http.StatusConflict, "Hymn number already exists", nil)
		return
	}

	versesStr, err := normalizeVerses(req.Verses)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid verses format", err.Error())
		return
	}

	hymn := models.Hymn{
		Number:   req.Number,
		Title:    strings.TrimSpace(req.Title),
		Chorus:   strings.TrimSpace(req.Chorus),
		Verses:   versesStr,
		Category: strings.TrimSpace(req.Category),
		Author:   strings.TrimSpace(req.Author),
		Key:      cleanKey(req.Key),
	}

	if err := db.DB.Create(&hymn).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to create hymn", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusCreated, "Hymn created successfully", hymn)
}

// AdminUpdateHymnHandler updates an existing hymn
func AdminUpdateHymnHandler(c *gin.Context) {
	idParam := c.Param("id")
	hymnUUID, err := uuid.Parse(idParam)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid hymn ID", err.Error())
		return
	}

	var hymn models.Hymn
	if err := db.DB.First(&hymn, "id = ?", hymnUUID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.SendError(c, http.StatusNotFound, "Hymn not found", nil)
			return
		}
		utils.SendError(c, http.StatusInternalServerError, "Failed to find hymn", err.Error())
		return
	}

	var req UpdateHymnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.SendError(c, http.StatusBadRequest, "Validation error", err.Error())
		return
	}

	if req.Number > 0 && req.Number != hymn.Number {
		var count int64
		db.DB.Model(&models.Hymn{}).Where("number = ? AND id != ?", req.Number, hymn.ID).Count(&count)
		if count > 0 {
			utils.SendError(c, http.StatusConflict, "Hymn number already in use by another hymn", nil)
			return
		}
		hymn.Number = req.Number
	}

	if req.Title != "" {
		hymn.Title = strings.TrimSpace(req.Title)
	}
	hymn.Chorus = strings.TrimSpace(req.Chorus)
	if req.Category != "" {
		hymn.Category = strings.TrimSpace(req.Category)
	}
	if req.Author != "" {
		hymn.Author = strings.TrimSpace(req.Author)
	}
	if req.Key != "" {
		hymn.Key = cleanKey(req.Key)
	}

	if req.Verses != nil {
		versesStr, vErr := normalizeVerses(req.Verses)
		if vErr == nil {
			hymn.Verses = versesStr
		}
	}

	if err := db.DB.Save(&hymn).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to update hymn", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Hymn updated successfully", hymn)
}

// AdminDeleteHymnHandler deletes a hymn
func AdminDeleteHymnHandler(c *gin.Context) {
	idParam := c.Param("id")
	hymnUUID, err := uuid.Parse(idParam)
	if err != nil {
		utils.SendError(c, http.StatusBadRequest, "Invalid hymn ID", err.Error())
		return
	}

	if err := db.DB.Delete(&models.Hymn{}, "id = ?", hymnUUID).Error; err != nil {
		utils.SendError(c, http.StatusInternalServerError, "Failed to delete hymn", err.Error())
		return
	}

	utils.SendSuccess(c, http.StatusOK, "Hymn deleted successfully", nil)
}

// AdminBulkUploadHymnsHandler parses a document file (DOCX, TXT, JSON) or JSON payload of hymns and inserts/updates them
func AdminBulkUploadHymnsHandler(c *gin.Context) {
	var parsedHymns []models.Hymn

	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		fileHeader, err := c.FormFile("file")
		if err != nil {
			utils.SendError(c, http.StatusBadRequest, "Form file 'file' is required", err.Error())
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			utils.SendError(c, http.StatusInternalServerError, "Failed to open uploaded file", err.Error())
			return
		}
		defer file.Close()

		fileBytes, err := io.ReadAll(file)
		if err != nil {
			utils.SendError(c, http.StatusInternalServerError, "Failed to read file", err.Error())
			return
		}

		// Use the flexible document parser
		parsed, err := services.ParseHymnDocument(fileHeader.Filename, fileBytes)
		if err != nil {
			utils.SendError(c, http.StatusUnprocessableEntity, "Failed to parse hymn document: "+err.Error(), nil)
			return
		}
		parsedHymns = parsed
	} else {
		var hymnsInput []CreateHymnRequest
		if err := c.ShouldBindJSON(&hymnsInput); err != nil {
			utils.SendError(c, http.StatusBadRequest, "Invalid JSON payload: "+err.Error(), nil)
			return
		}
		for _, item := range hymnsInput {
			vStr, _ := normalizeVerses(item.Verses)
			parsedHymns = append(parsedHymns, models.Hymn{
				Number:   item.Number,
				Title:    strings.TrimSpace(item.Title),
				Chorus:   strings.TrimSpace(item.Chorus),
				Verses:   vStr,
				Category: strings.TrimSpace(item.Category),
				Author:   strings.TrimSpace(item.Author),
				Key:      strings.TrimSpace(item.Key),
			})
		}
	}

	if len(parsedHymns) == 0 {
		utils.SendError(c, http.StatusBadRequest, "No valid hymns found in uploaded document", nil)
		return
	}

	var insertedCount int
	var updatedCount int
	var errorMessages []string

	for _, item := range parsedHymns {
		if item.Number <= 0 || strings.TrimSpace(item.Title) == "" {
			continue
		}

		var existing models.Hymn
		findErr := db.DB.Unscoped().Where("number = ?", item.Number).First(&existing).Error
		if findErr == nil {
			existing.DeletedAt = gorm.DeletedAt{}
			// Update existing without overwriting good content with empty/null
			if item.Title != "" && item.Title != "Hymn "+strconv.Itoa(item.Number) {
				existing.Title = item.Title
			} else if existing.Title == "" {
				existing.Title = item.Title
			}
			if item.Chorus != "" {
				existing.Chorus = item.Chorus
			}
			if item.Verses != "" && item.Verses != "[]" && item.Verses != "null" {
				existing.Verses = item.Verses
			}
			if item.Category != "" {
				existing.Category = item.Category
			}
			if item.Author != "" {
				existing.Author = item.Author
			}
			if item.Key != "" {
				existing.Key = cleanKey(item.Key)
			}
			if saveErr := db.DB.Save(&existing).Error; saveErr == nil {
				updatedCount++
			} else {
				errorMessages = append(errorMessages, "Hymn #"+strconv.Itoa(item.Number)+": "+saveErr.Error())
			}
		} else {
			// Insert new
			if item.ID == uuid.Nil {
				item.ID = uuid.New()
			}
			item.Key = cleanKey(item.Key)
			if createErr := db.DB.Create(&item).Error; createErr == nil {
				insertedCount++
			} else {
				errorMessages = append(errorMessages, "Hymn #"+strconv.Itoa(item.Number)+": "+createErr.Error())
			}
		}
	}

	utils.SendSuccess(c, http.StatusOK, "Bulk import completed", gin.H{
		"inserted": insertedCount,
		"updated":  updatedCount,
		"total":    len(parsedHymns),
		"count":    insertedCount + updatedCount,
		"errors":   errorMessages,
	})
}

// SeedDefaultHymns populates classic hymns and repairs corrupted hymns or double keys
func SeedDefaultHymns() {
	defaults := []models.Hymn{
		{
			Number:   1,
			Title:    "Holy, Holy, Holy! Lord God Almighty!",
			Author:   "Reginald Heber (1826)",
			Key:      "E Major (NICAEA)",
			Category: "Adoration and Praise",
			Verses: `[
				"Holy, holy, holy! Lord God Almighty!\nEarly in the morning our song shall rise to Thee;\nHoly, holy, holy! Merciful and mighty!\nGod in three Persons, blessed Trinity!",
				"Holy, holy, holy! All the saints adore Thee,\nCasting down their golden crowns around the glassy sea;\nCherubim and seraphim falling down before Thee,\nWhich wert, and art, and evermore shalt be.",
				"Holy, holy, holy! Though the darkness hide Thee,\nThough the eye of sinful man Thy glory may not see;\nOnly Thou art holy; there is none beside Thee,\nPerfect in power, in love, and purity.",
				"Holy, holy, holy! Lord God Almighty!\nAll Thy works shall praise Thy name, in earth, and sky, and sea;\nHoly, holy, holy! Merciful and mighty!\nGod in three Persons, blessed Trinity!"
			]`,
		},
		{
			Number:   2,
			Title:    "Praise, My Soul, the King of Heaven",
			Author:   "Henry F. Lyte (1834)",
			Key:      "D Major (PRAISE MY SOUL)",
			Category: "Adoration and Praise",
			Chorus:   "Praise Him! Praise Him! Praise Him! Praise Him!\nPraise the everlasting King.",
			Verses: `[
				"Praise, my soul, the King of heaven;\nTo His feet thy tribute bring;\nRansomed, healed, restored, forgiven,\nWho like thee His praise should sing?",
				"Praise Him for His grace and favour\nTo our fathers in distress;\nPraise Him still the same forever,\nSlow to chide and swift to bless.",
				"Father-like, He tends and spares us;\nWell our feeble frame He knows;\nIn His hands He gently bears us,\nRescues us from all our foes.",
				"Frail as summer's flower we flourish,\nBlows the wind and it is gone;\nBut while mortals rise and perish,\nGod endures unchanging on.",
				"Angels, help us to adore Him;\nYe behold Him face to face;\nSun and moon, bow down before Him,\nDwellers all in time and space."
			]`,
		},
		{
			Number:   3,
			Title:    "Amazing Grace",
			Author:   "John Newton (1779)",
			Key:      "G Major (NEW BRITAIN)",
			Category: "Atonement and Grace",
			Verses: `[
				"Amazing grace! How sweet the sound\nThat saved a wretch like me!\nI once was lost, but now am found;\nWas blind, but now I see.",
				"'Twas grace that taught my heart to fear,\nAnd grace my fears relieved;\nHow precious did that grace appear\nThe hour I first believed.",
				"Through many dangers, toils and snares,\nI have already come;\n'Tis grace hath brought me safe thus far,\nAnd grace will lead me home.",
				"The Lord has promised good to me,\nHis Word my hope secures;\nHe will my Shield and Portion be,\nAs long as life endures.",
				"When we've been there ten thousand years,\nBright shining as the sun,\nWe've no less days to sing God's praise\nThan when we'd first begun."
			]`,
		},
		{
			Number:   4,
			Title:    "Great Is Thy Faithfulness",
			Author:   "Thomas O. Chisholm (1923)",
			Key:      "D Major (FAITHFULNESS)",
			Category: "Faith and Trust",
			Chorus:   "Great is Thy faithfulness! Great is Thy faithfulness!\nMorning by morning new mercies I see;\nAll I have needed Thy hand hath provided—\nGreat is Thy faithfulness, Lord, unto me!",
			Verses: `[
				"Great is Thy faithfulness, O God my Father,\nThere is no shadow of turning with Thee;\nThou changest not, Thy compassions, they fail not;\nAs Thou hast been Thou forever wilt be.",
				"Summer and winter, and springtime and harvest,\nSun, moon and stars in their courses above,\nJoin with all nature in manifold witness\nTo Thy great faithfulness, mercy and love.",
				"Pardon for sin and a peace that endureth,\nThine own dear presence to cheer and to guide;\nStrength for today and bright hope for tomorrow,\nBlessings all mine, with ten thousand beside!"
			]`,
		},
		{
			Number:   5,
			Title:    "Blessed Assurance",
			Author:   "Fanny J. Crosby (1873)",
			Key:      "D Major (ASSURANCE)",
			Category: "Assurance",
			Chorus:   "This is my story, this is my song,\nPraising my Savior all the day long;\nThis is my story, this is my song,\nPraising my Savior all the day long.",
			Verses: `[
				"Blessed assurance, Jesus is mine!\nO what a foretaste of glory divine!\nHier of salvation, purchase of God,\nBorn of His Spirit, washed in His blood.",
				"Perfect submission, perfect delight,\nVisions of rapture now burst on my sight;\nAngels descending, bring from above\nEchoes of mercy, whispers of love.",
				"Perfect submission, all is at rest,\nI in my Savior am happy and blest,\nWatching and waiting, looking above,\nFilled with His goodness, lost in His love."
			]`,
		},
		{
			Number:   6,
			Title:    "It Is Well With My Soul",
			Author:   "Horatio G. Spafford (1873)",
			Key:      "C Major (VILLE DU HAVRE)",
			Category: "Peace and Comfort",
			Chorus:   "It is well with my soul,\nIt is well, it is well with my soul.",
			Verses: `[
				"When peace like a river, attendeth my way,\nWhen sorrows like sea billows roll;\nWhatever my lot, Thou hast taught me to say,\nIt is well, it is well with my soul.",
				"Though Satan should buffet, though trials should come,\nLet this blest assurance control,\nThat Christ has regarded my helpless estate,\nAnd hath shed His own blood for my soul.",
				"My sin—oh, the bliss of this glorious thought!—\nMy sin, not in part but the whole,\nIs nailed to the cross, and I bear it no more,\nPraise the Lord, praise the Lord, O my soul!",
				"And Lord, haste the day when my faith shall be sight,\nThe clouds be rolled back as a scroll;\nThe trump shall resound, and the Lord shall descend,\nEven so, it is well with my soul."
			]`,
		},
		{
			Number:   7,
			Title:    "How Great Thou Art",
			Author:   "Stuart K. Hine (1949)",
			Key:      "Bb Major (O STORE GUD)",
			Category: "Adoration and Praise",
			Chorus:   "Then sings my soul, my Savior God, to Thee,\nHow great Thou art! How great Thou art!\nThen sings my soul, my Savior God, to Thee,\nHow great Thou art! How great Thou art!",
			Verses: `[
				"O Lord my God, when I in awesome wonder,\nConsider all the worlds Thy hands have made;\nI see the stars, I hear the rolling thunder,\nThy power throughout the universe displayed.",
				"When through the woods, and forest glades I wander,\nAnd hear the birds sing sweetly in the trees;\nWhen I look down, from lofty mountain grandeur\nAnd see the brook, and feel the gentle breeze.",
				"And when I think, that God, His Son not sparing;\nSent Him to die, I scarce can take it in;\nThat on the cross, my burden gladly bearing,\nHe bled and died to take away my sin.",
				"When Christ shall come, with shout of acclamation,\nAnd take me home, what joy shall fill my heart!\nThen I shall bow, in humble adoration,\nAnd then proclaim: 'My God, how great Thou art!'"
			]`,
		},
		{
			Number:   8,
			Title:    "Praise Him! Praise Him!",
			Author:   "Fanny J. Crosby (1869)",
			Key:      "Ab Major (ALLEN)",
			Category: "Adoration and Praise",
			Chorus:   "Praise Him! Praise Him! Tell of His excellent greatness;\nPraise Him! Praise Him! Ever in joyful song!",
			Verses: `[
				"Praise Him! Praise Him! Jesus, our blessed Redeemer!\nSing, O Earth, His wonderful love proclaim!\nHail Him! Hail Him! Highest archangels in glory;\nStrength and honor give to His holy name!\nLike a shepherd, Jesus will guard His children,\nIn His arms He carries them all day long.",
				"Praise Him! Praise Him! Jesus, our blessed Redeemer!\nFor our sins He suffered, and bled, and died;\nHe, our Rock, our hope of eternal salvation,\nHail Him! Hail Him! Jesus the Crucified.\nSound His praises! Jesus who bore our sorrows,\nLove unbounded, wonderful, deep and strong.",
				"Praise Him! Praise Him! Jesus, our blessed Redeemer!\nHeavenly portals loud with hosannas ring!\nJesus, Savior, reigneth forever and ever;\nCrown Him! Crown Him! Prophet, and Priest, and King!\nChrist is coming! Over the globe victorious,\nPower and glory unto the Lord belong."
			]`,
		},
		{
			Number:   9,
			Title:    "Rock of Ages, Cleft for Me",
			Author:   "Augustus M. Toplady (1776)",
			Key:      "Bb Major (TOPLADY)",
			Category: "Atonement and Grace",
			Verses: `[
				"Rock of Ages, cleft for me,\nLet me hide myself in Thee;\nLet the water and the blood,\nFrom Thy wounded side which flowed,\nBe of sin the double cure,\nSave from wrath and make me pure.",
				"Not the labors of my hands\nCan fulfill Thy law's demands;\nCould my zeal no respite know,\nCould my tears forever flow,\nAll for sin could not atone;\nThou must save, and Thou alone.",
				"Nothing in my hand I bring,\nSimply to the cross I cling;\nNaked, come to Thee for dress;\nHelpless, look to Thee for grace;\nFoul, I to the fountain fly;\nWash me, Savior, or I die.",
				"While I draw this fleeting breath,\nWhen mine eyes shall close in death,\nWhen I soar to worlds unknown,\nSee Thee on Thy judgment throne,\nRock of Ages, cleft for me,\nLet me hide myself in Thee."
			]`,
		},
		{
			Number:   10,
			Title:    "O Worship the King",
			Author:   "Robert Grant (1833)",
			Key:      "G Major (LYONS)",
			Category: "Adoration and Praise",
			Verses: `[
				"O worship the King, all glorious above,\nO gratefully sing His power and His love;\nOur Shield and Defender, the Ancient of Days,\nPavilion'd in splendor, and girded with praise.",
				"O tell of His might, O sing of His grace,\nWhose robe is the light, whose canopy space;\nHis chariots of wrath the deep thunderclouds form,\nAnd dark is His path on the wings of the storm.",
				"Thy bountiful care, what tongue can recite?\nIt breathes in the air, it shines in the light;\nIt streams from the hills, it descends to the plain,\nAnd sweetly distils in the dew and the rain.",
				"Frail children of dust, and feeble as frail,\nIn Thee do we trust, nor find Thee to fail;\nThy mercies, how tender, how firm to the end,\nOur Maker, Defender, Redeemer, and Friend!"
			]`,
		},
	}

	for _, hymn := range defaults {
		var existing models.Hymn
		err := db.DB.Where("number = ?", hymn.Number).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hymn.ID = uuid.New()
			db.DB.Create(&hymn)
		} else if err == nil {
			needsUpdate := false
			if existing.Verses == "null" || existing.Verses == "[]" || existing.Verses == "" || existing.Title == "Hymn "+strconv.Itoa(existing.Number) {
				existing.Title = hymn.Title
				existing.Verses = hymn.Verses
				existing.Chorus = hymn.Chorus
				existing.Author = hymn.Author
				existing.Category = hymn.Category
				needsUpdate = true
			}
			cleaned := cleanKey(existing.Key)
			if cleaned != existing.Key {
				existing.Key = cleaned
				needsUpdate = true
			}
			if needsUpdate {
				db.DB.Save(&existing)
			}
		}
	}
}

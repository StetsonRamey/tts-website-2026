/**
 * Google Places Autocomplete (New API)
 * Uses AutocompleteSuggestion.fetchAutocompleteSuggestions + Place.fetchFields
 * Falls back to plain text input if Google API fails
 */
(function () {
  "use strict";

  var addressInput = document.getElementById("street-address");
  if (!addressInput) return;

  var cityInput = document.getElementById("city");
  var stateInput = document.getElementById("state");
  var zipInput = document.getElementById("zip");

  if (!cityInput || !stateInput || !zipInput) return;

  // Google state name → abbreviation
  var STATE_ABBREV = {
    Missouri: "MO",
    Kansas: "KS",
  };

  /**
   * Extract a specific address component from Google's result (new API)
   */
  function getComponent(addressComponents, type) {
    if (!addressComponents) return "";
    for (var i = 0; i < addressComponents.length; i++) {
      var component = addressComponents[i];
      if (component.types.indexOf(type) !== -1) {
        return component.longText || component.shortText || "";
      }
    }
    return "";
  }

  /**
   * Handle place selection from autocomplete
   */
  function onPlaceSelected(place) {
    if (!place) return;

    var addressComponents = place.addressComponents || [];

    // Build street address
    var streetNumber = getComponent(addressComponents, "street_number");
    var route = getComponent(addressComponents, "route");
    var street = streetNumber && route ? streetNumber + " " + route : route || "";
    if (street) addressInput.value = street;

    // City
    var city = getComponent(addressComponents, "locality");
    if (city) cityInput.value = city;

    // State
    var stateName = getComponent(addressComponents, "administrative_area_level_1");
    var stateAbbr = STATE_ABBREV[stateName] || stateName;
    if (stateAbbr) {
      for (var i = 0; i < stateInput.options.length; i++) {
        if (stateInput.options[i].value === stateAbbr) {
          stateInput.value = stateAbbr;
          break;
        }
      }
    }

    // ZIP
    var zip = getComponent(addressComponents, "postal_code");
    if (zip) zipInput.value = zip;

    cityInput.focus();
    cityInput.select();
  }

  /**
   * Fetch place details and fill form fields
   */
  function selectPlace(placePrediction, sessionToken) {
    var place = placePrediction.toPlace();
    place
      .fetchFields({
        fields: ["addressComponents", "formattedAddress"],
        sessionToken: sessionToken,
      })
      .then(function (result) {
        onPlaceSelected(result.place);
      })
      .catch(function (err) {
        console.warn("Place fetchFields error:", err);
      });
  }

  /**
   * Initialize Places Autocomplete
   */
  function initAutocomplete() {
    if (!window.google || !window.google.maps || !window.google.maps.places) {
      return;
    }

    var sessionToken = new window.google.maps.places.AutocompleteSessionToken();
    var inputTimeout;
    var selectedIndex = -1;
    var currentSuggestions = [];

    /**
     * Update highlight on selected item
     */
    function updateHighlight() {
      var items = document.querySelectorAll("#autocomplete-dropdown > div");
      items.forEach(function (item, index) {
        if (index === selectedIndex) {
          item.style.backgroundColor = "#f0f0f0";
        } else {
          item.style.backgroundColor = "transparent";
        }
      });
    }

    /**
     * Show predictions dropdown
     */
    function showPredictions(suggestions) {
      var container = document.getElementById("autocomplete-dropdown");
      if (!container) {
        container = document.createElement("div");
        container.id = "autocomplete-dropdown";
        var inputRect = addressInput.getBoundingClientRect();
        var topOffset = inputRect.height + 40;

        container.style.cssText =
          "position: absolute; top: " + topOffset + "px; left: 0; right: 0; z-index: 1000; background: white; border: 1px solid #ccc; border-radius: 4px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); max-height: 250px; overflow-y: auto;";
        addressInput.parentNode.style.position = "relative";
        addressInput.parentNode.appendChild(container);
      }

      container.innerHTML = "";
      if (!suggestions || suggestions.length === 0) {
        container.style.display = "none";
        return;
      }

      suggestions.forEach(function (suggestion, index) {
        var prediction = suggestion.placePrediction;
        if (!prediction) return;

        var item = document.createElement("div");
        item.textContent = prediction.text.text;
        item.dataset.index = index;
        item.style.cssText =
          "padding: 10px; cursor: pointer; border-bottom: 1px solid #eee; transition: background-color 0.15s;";

        item.onmouseenter = function () {
          selectedIndex = index;
          updateHighlight();
        };
        item.onmouseleave = function () {
          item.style.backgroundColor = "transparent";
        };
        item.onclick = function () {
          addressInput.value = prediction.text.text;
          container.style.display = "none";
          selectedIndex = -1;
          selectPlace(prediction, sessionToken);
          sessionToken = new window.google.maps.places.AutocompleteSessionToken();
        };
        container.appendChild(item);
      });

      container.style.display = "block";
    }

    /**
     * Handle keyboard navigation
     */
    addressInput.addEventListener("keydown", function (e) {
      var container = document.getElementById("autocomplete-dropdown");
      var items = container ? document.querySelectorAll("#autocomplete-dropdown > div") : [];

      if (e.key === "ArrowDown") {
        e.preventDefault();
        selectedIndex = Math.min(selectedIndex + 1, items.length - 1);
        updateHighlight();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        selectedIndex = Math.max(selectedIndex - 1, -1);
        updateHighlight();
      } else if (e.key === "Enter" && selectedIndex >= 0) {
        e.preventDefault();
        var selectedItem = items[selectedIndex];
        var suggestion = currentSuggestions[selectedIndex];
        if (suggestion && suggestion.placePrediction) {
          addressInput.value = suggestion.placePrediction.text.text;
          selectPlace(suggestion.placePrediction, sessionToken);
          sessionToken = new window.google.maps.places.AutocompleteSessionToken();
        }
        if (container) container.style.display = "none";
        selectedIndex = -1;
      }
    });

    /**
     * Request predictions on input change
     */
    addressInput.addEventListener("input", function (e) {
      selectedIndex = -1;
      clearTimeout(inputTimeout);
      var input = e.target.value.trim();

      if (input.length < 3) {
        var dropdown = document.getElementById("autocomplete-dropdown");
        if (dropdown) dropdown.style.display = "none";
        return;
      }

      inputTimeout = setTimeout(function () {
        window.google.maps.places.AutocompleteSuggestion.fetchAutocompleteSuggestions({
          input: input,
          includedRegionCodes: ["us"],
          sessionToken: sessionToken,
        })
          .then(function (result) {
            currentSuggestions = result.suggestions || [];
            showPredictions(currentSuggestions);
          })
          .catch(function (err) {
            console.warn("AutocompleteSuggestion error:", err);
            var dropdown = document.getElementById("autocomplete-dropdown");
            if (dropdown) dropdown.style.display = "none";
          });
      }, 300);
    });

    // Close dropdown on blur
    addressInput.addEventListener("blur", function () {
      setTimeout(function () {
        var container = document.getElementById("autocomplete-dropdown");
        if (container) container.style.display = "none";
      }, 200);
    });
  }

  /**
   * Initialize when Google API is ready
   */
  window.initAddressAutocomplete = function () {
    initAutocomplete();
  };

  // If script already loaded, init immediately
  if (window.google && window.google.maps && window.google.maps.places) {
    initAutocomplete();
  }
})();

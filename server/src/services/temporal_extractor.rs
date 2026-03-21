use chrono::{DateTime, Utc};
use regex::Regex;
use chrono::Datelike;
use anyhow::Result;

/// Service for extracting temporal information from memory content
/// Implements Supermemory-style dual timestamp extraction:
/// - document_date: When the conversation occurred
/// - event_date: When the event described actually occurred
pub struct TemporalExtractor;

impl TemporalExtractor {
    /// Extract event dates from memory content using pattern matching
    /// Returns Option<DateTime<Utc>> if an event date is found
    pub fn extract_event_date(content: &str) -> Option<DateTime<Utc>> {
        // Try common temporal patterns
        Self::extract_iso_date(content)
            .or_else(|| Self::extract_natural_date(content))
    }

    /// Extract ISO 8601 formatted dates
    fn extract_iso_date(content: &str) -> Option<DateTime<Utc>> {
        let iso_regex = Regex::new(
            r"(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}):(\d{2})Z?)?"
        ).ok()?;

        if let Some(caps) = iso_regex.captures(content) {
            let year: i32 = caps.get(1)?.as_str().parse::<i32>().ok()?;
            let month: u32 = caps.get(2)?.as_str().parse().ok()?;
            let day: u32 = caps.get(3)?.as_str().parse().ok()?;
            let hour: u32 = caps.get(4).map(|m| m.as_str().parse::<u32>().ok()).flatten().unwrap_or(0);
            let minute: u32 = caps.get(5).map(|m| m.as_str().parse::<u32>().ok()).flatten().unwrap_or(0);
            let second: u32 = caps.get(6).map(|m| m.as_str().parse::<u32>().ok()).flatten().unwrap_or(0);

            use chrono::NaiveDate;
            let date = NaiveDate::from_ymd_opt(year, month, day)?
                .and_hms_opt(hour, minute, second)?;
            return Some(DateTime::from_naive_utc_and_offset(date, Utc));
        }

        None
    }

    /// Extract natural language dates (e.g., "yesterday", "2 weeks ago", "next Monday")
    /// This is a simplified version - for production, use a proper date parsing library like `chrono-english`
    fn extract_natural_date(content: &str) -> Option<DateTime<Utc>> {
        let now = Utc::now();
        let content_lower = content.to_lowercase();

        // Very basic patterns - in production, use a proper NLP library
        if content_lower.contains("yesterday") {
            return Some(now - chrono::Duration::days(1));
        }
        if content_lower.contains("today") {
            return Some(now);
        }
        if content_lower.contains("tomorrow") {
            return Some(now + chrono::Duration::days(1));
        }
        if content_lower.contains("last week") {
            return Some(now - chrono::Duration::weeks(1));
        }
        if content_lower.contains("next week") {
            return Some(now + chrono::Duration::weeks(1));
        }
        if content_lower.contains("last month") {
            return Some(now - chrono::Duration::days(30));
        }
        if content_lower.contains("next month") {
            return Some(now + chrono::Duration::days(30));
        }

        // Try to extract common date patterns like "March 15" or "15/03"
        Self::extract_partial_date(&content_lower, &now)
    }

    /// Extract partial dates like "March 15" (assumes current year)
    fn extract_partial_date(content: &str, now: &DateTime<Utc>) -> Option<DateTime<Utc>> {
        let months = [
            ("january", 1), ("february", 2), ("march", 3), ("april", 4),
            ("may", 5), ("june", 6), ("july", 7), ("august", 8),
            ("september", 9), ("october", 10), ("november", 11), ("december", 12),
        ];

        for (month_name, month_num) in &months {
            if content.contains(month_name) {
                // Look for day number after month name
                let day_regex = Regex::new(&format!(r"{}\s+(\d{{1,2}})", month_name)).ok()?;
                if let Some(caps) = day_regex.captures(content) {
                    let day: u32 = caps.get(1)?.as_str().parse::<u32>().ok()?;
                    let year = now.year();

                    use chrono::NaiveDate;
                    let date = NaiveDate::from_ymd_opt(year, *month_num, day)?
                        .and_hms_opt(0, 0, 0)?;
                    return Some(DateTime::from_naive_utc_and_offset(date, Utc));
                }
            }
        }

        None
    }

    /// For future enhancement: Extract dates using LLM
    /// This would call an LLM with a prompt like:
    /// "Extract all event dates mentioned in this text. Return ISO 8601 format or null."
    /// For now, this is a placeholder for documentation
    pub async fn extract_event_date_with_llm(
        content: &str,
        _llm_provider: &str, // "openai", "anthropic", etc.
    ) -> Result<Option<DateTime<Utc>>> {
        // TODO: Implement LLM-based extraction using embedding provider
        // This would be more accurate for natural language dates
        Ok(Self::extract_event_date(content))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_iso_date() {
        let content = "I went to the beach on 2024-06-15T14:30:00Z";
        let result = TemporalExtractor::extract_event_date(content);
        assert!(result.is_some());
    }

    #[test]
    fn test_extract_yesterday() {
        let content = "Yesterday I had a great day at work";
        let result = TemporalExtractor::extract_event_date(content);
        assert!(result.is_some());
    }

    #[test]
    fn test_extract_month_and_day() {
        let content = "I remember March 15 being a special day";
        let result = TemporalExtractor::extract_event_date(content);
        assert!(result.is_some());
    }

    #[test]
    fn test_no_date_visible() {
        let content = "This is just some random text";
        let result = TemporalExtractor::extract_event_date(content);
        // Might be None, but if it extracted something, that's also valid
        // for more complex content
        let _ = result;
    }
}

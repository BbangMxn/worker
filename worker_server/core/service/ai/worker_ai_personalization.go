package ai

import (
	"context"

	"worker_server/core/domain"

	"github.com/google/uuid"
)

type PersonalizationService struct {
	profileRepo domain.ProfileRepository
	emailRepo   domain.EmailRepository
}

func NewPersonalizationService(profileRepo domain.ProfileRepository, emailRepo domain.EmailRepository) *PersonalizationService {
	return &PersonalizationService{
		profileRepo: profileRepo,
		emailRepo:   emailRepo,
	}
}

func (s *PersonalizationService) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	return s.profileRepo.GetByUserID(userID)
}

func (s *PersonalizationService) AnalyzeWritingStyle(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	// Get sent emails
	folder := domain.LegacyFolderSent
	filter := &domain.EmailFilter{
		UserID: userID,
		Folder: &folder,
		Limit:  100, // Analyze last 100 sent emails
	}

	emails, _, err := s.emailRepo.List(filter)
	if err != nil {
		return nil, err
	}

	if len(emails) == 0 {
		return nil, nil
	}

	// Analyze patterns
	profile, err := s.profileRepo.GetByUserID(userID)
	if err != nil {
		profile = &domain.UserProfile{
			UserID: userID,
		}
	}

	// TODO: Implement actual analysis with LLM
	profile.TotalEmailsAnalyzed = len(emails)

	if profile.ID == 0 {
		if err := s.profileRepo.Create(profile); err != nil {
			return nil, err
		}
	} else {
		if err := s.profileRepo.Update(profile); err != nil {
			return nil, err
		}
	}

	return profile, nil
}

func (s *PersonalizationService) GetCommonPhrases(ctx context.Context, userID uuid.UUID) ([]string, error) {
	profile, err := s.profileRepo.GetByUserID(userID)
	if err != nil {
		return nil, err
	}
	return profile.CommonPhrases, nil
}

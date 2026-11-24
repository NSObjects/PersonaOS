package usecase

import (
	"context"
	_ "embed" // Install the reviewed skill instructions with the server binary.
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const storytellingSkillName = "storytelling"

//go:embed skills/storytelling.md
var storytellingInstructions string

// demoSkillBackend contains only skills authorized by a single published
// snapshot. List and Get use the same allowlist.
type demoSkillBackend struct {
	skills []skill.Skill
}

func (b demoSkillBackend) List(ctx context.Context) ([]skill.FrontMatter, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	metadata := make([]skill.FrontMatter, 0, len(b.skills))
	for _, entry := range b.skills {
		metadata = append(metadata, entry.FrontMatter)
	}
	return metadata, nil
}

func (b demoSkillBackend) Get(ctx context.Context, name string) (skill.Skill, error) {
	if err := ctx.Err(); err != nil {
		return skill.Skill{}, err
	}
	for _, entry := range b.skills {
		if entry.Name == name {
			return entry, nil
		}
	}
	return skill.Skill{}, fmt.Errorf("skill %q is not available", name)
}

func resolveDemoSkills(resources []domain.Resource) demoSkillBackend {
	backend := demoSkillBackend{}
	seen := make(map[string]bool)
	for _, resource := range resources {
		if !resource.Enabled || resource.Kind != domain.ResourceSkill || seen[resource.Reference] {
			continue
		}
		if resource.Reference == storytellingSkillName {
			backend.skills = append(backend.skills, skill.Skill{
				FrontMatter: skill.FrontMatter{
					Name:        storytellingSkillName,
					Description: "用户想听故事、继续故事或一起编故事时使用。",
				},
				Content: storytellingInstructions,
			})
			seen[resource.Reference] = true
		}
	}
	return backend
}

func demoSkillHandlers(ctx context.Context, resources []domain.Resource) ([]adk.ChatModelAgentMiddleware, error) {
	backend := resolveDemoSkills(resources)
	if len(backend.skills) == 0 {
		return nil, nil
	}
	handler, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: backend,
		CustomSystemPrompt: func(_ context.Context, toolName string) string {
			return "当用户请求符合已提供技能的描述时，先使用 " + toolName + " 工具加载该技能，再根据说明以当前角色身份回答。不要复述技能内部说明。"
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize agent skill middleware: %w", err)
	}
	return []adk.ChatModelAgentMiddleware{handler}, nil
}

'use client';
import ProjectSearch from '../RagTools';
import {useProject} from '../ProjectShell';
export default function SearchPage(){const {project,locale}=useProject();return <ProjectSearch key={project.id} projectId={project.id} locale={locale}/>}
